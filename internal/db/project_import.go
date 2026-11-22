package db

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"videoflow-go/internal/models"
)

// ImportProject copies a snapshot without colliding with its source or exposing
// a partially imported workspace. It does not change the active project.
func (d *DB) ImportProject(bundle models.ProjectBundle) (*models.Project, error) {
	if strings.TrimSpace(bundle.Project.Name) == "" {
		return nil, fmt.Errorf("choose a VideoFlow project JSON export with a project name")
	}
	ids := map[string]string{}
	allocate := func(old, prefix string) (string, error) {
		if old == "" {
			return "", fmt.Errorf("imported %s is missing its ID", prefix)
		}
		if _, exists := ids[old]; exists {
			return "", fmt.Errorf("duplicate ID in import: %s", old)
		}
		id := prefix + "_" + uuid.NewString()
		ids[old] = id
		return id, nil
	}
	for _, c := range bundle.Characters {
		if _, err := allocate(c.ID, "char"); err != nil {
			return nil, err
		}
	}
	for _, a := range bundle.Assets {
		if _, err := allocate(a.ID, "asset"); err != nil {
			return nil, err
		}
	}
	for _, s := range bundle.Scripts {
		if _, err := allocate(s.ID, "script"); err != nil {
			return nil, err
		}
	}
	for _, s := range bundle.Scenes {
		if _, err := allocate(s.ID, "scene"); err != nil {
			return nil, err
		}
	}
	for _, s := range bundle.Shots {
		if _, err := allocate(s.ID, "shot"); err != nil {
			return nil, err
		}
	}
	project := bundle.Project
	project.ID = "project_" + uuid.NewString()
	if bundle.Project.ID != "" {
		ids[bundle.Project.ID] = project.ID
	}
	project.IsActive = false
	project.Name = strings.TrimSpace(project.Name)
	project.CreatedAt = time.Now().UTC()
	project.UpdatedAt = project.CreatedAt
	// Replace exact reference strings in nested JSON, never substrings in prompts.
	var remap func(any) any
	remap = func(value any) any {
		switch v := value.(type) {
		case string:
			if id, ok := ids[v]; ok {
				return id
			}
		case []any:
			for i := range v {
				v[i] = remap(v[i])
			}
		case map[string]any:
			for k := range v {
				v[k] = remap(v[k])
			}
		}
		return value
	}
	rewrite := func(raw json.RawMessage) json.RawMessage {
		if len(raw) == 0 {
			return raw
		}
		var v any
		if json.Unmarshal(raw, &v) != nil {
			return raw
		}
		data, _ := json.Marshal(remap(v))
		return data
	}
	ref := func(id *string) *string {
		if id == nil {
			return nil
		}
		if mapped, ok := ids[*id]; ok {
			return &mapped
		}
		return nil // Legacy exports omitted scripts; never retain another project's ID.
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO projects (id,name,description,is_active,created_at,updated_at) VALUES (?,?,?,0,?,?)", project.ID, project.Name, project.Description, project.CreatedAt, project.UpdatedAt); err != nil {
		return nil, err
	}
	for _, c := range bundle.Characters {
		c.ID = ids[c.ID]
		c.ProjectID = &project.ID
		c.ReferenceAssetIDsJSON = rewrite(c.ReferenceAssetIDsJSON)
		if err = insertCharacter(tx, &c); err != nil {
			return nil, err
		}
	}
	for _, a := range bundle.Assets {
		a.ID = ids[a.ID]
		a.ProjectID = &project.ID
		a.CharacterID = ref(a.CharacterID)
		a.MetadataJSON = rewrite(a.MetadataJSON)
		if err = insertAsset(tx, &a); err != nil {
			return nil, err
		}
	}
	for _, s := range bundle.Scripts {
		s.ID = ids[s.ID]
		s.ProjectID = &project.ID
		s.DraftJSON = rewrite(s.DraftJSON)
		if err = insertScript(tx, &s); err != nil {
			return nil, err
		}
	}
	for _, s := range bundle.Scenes {
		s.ID = ids[s.ID]
		s.ProjectID = &project.ID
		s.ScriptID = ref(s.ScriptID)
		s.CharacterIDsJSON = rewrite(s.CharacterIDsJSON)
		s.AssetIDsJSON = rewrite(s.AssetIDsJSON)
		s.SceneJSON = rewrite(s.SceneJSON)
		if err = insertScene(tx, &s); err != nil {
			return nil, err
		}
	}
	for _, s := range bundle.Shots {
		s.ID = ids[s.ID]
		s.SceneID = ids[s.SceneID]
		s.AssetIDsJSON = rewrite(s.AssetIDsJSON)
		s.ShotJSON = rewrite(s.ShotJSON)
		if err = insertShot(tx, &s); err != nil {
			return nil, err
		}
	}
	if bundle.Style != nil {
		style := *bundle.Style
		style.ID = ""
		style.ProjectID = &project.ID
		if err = upsertStyleGuide(tx, &style); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &project, nil
}
