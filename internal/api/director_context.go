package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"videoflow-go/internal/models"
)

// Selection order is significant: it fixes reference-to-entity bindings for proposals.
type directorSelection struct {
	CharacterIDs []string `json:"character_ids"`
	AssetIDs     []string `json:"asset_ids"`
}
type directorContext struct {
	Selection  *directorSelection `json:"selection,omitempty"`
	ProjectID  string             `json:"project_id"`
	Revision   string             `json:"revision"`
	Characters []models.Character `json:"characters"`
	Assets     []directorAsset    `json:"assets"`
	Warnings   []string           `json:"warnings"`
}
type directorAsset struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	Description   string            `json:"description"`
	CharacterID   *string           `json:"character_id"`
	Tags          json.RawMessage   `json:"tags"`
	LockedTraits  map[string]string `json:"locked_traits"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Available     bool              `json:"available"`
	ContentSHA256 string            `json:"content_sha256"`
}

func (s *Server) buildDirectorContext(project string, selection directorSelection) (*directorContext, error) {
	if project == "" {
		return nil, fmt.Errorf("project is required")
	}
	if len(selection.CharacterIDs) > 8 || len(selection.AssetIDs) > 32 {
		return nil, fmt.Errorf("choose at most 8 characters and 32 explicit assets")
	}
	result := &directorContext{ProjectID: project, Characters: []models.Character{}, Assets: []directorAsset{}, Warnings: []string{}}
	seenCharacters := map[string]bool{}
	assetIDs := append([]string{}, selection.AssetIDs...)
	for _, id := range selection.CharacterIDs {
		if seenCharacters[id] {
			continue
		}
		seenCharacters[id] = true
		c, err := s.database.GetCharacter(id)
		if err != nil || c.ProjectID == nil || *c.ProjectID != project {
			return nil, fmt.Errorf("selected character is not in this project")
		}
		result.Characters = append(result.Characters, *c)
		var refs []string
		if json.Unmarshal(c.ReferenceAssetIDsJSON, &refs) != nil {
			return nil, fmt.Errorf("character has invalid reference bindings")
		}
		assetIDs = append(assetIDs, refs...)
	}
	seenAssets := map[string]bool{}
	for _, id := range assetIDs {
		if seenAssets[id] {
			continue
		}
		seenAssets[id] = true
		if len(seenAssets) > 64 {
			return nil, fmt.Errorf("selected context exceeds 64 reference assets; choose fewer references")
		}
		asset, err := s.database.GetAsset(id)
		if err != nil || asset.ProjectID == nil || *asset.ProjectID != project {
			return nil, fmt.Errorf("selected reference asset is not in this project")
		}
		var metadata struct {
			LockedTraits map[string]string `json:"locked_traits"`
		}
		if len(asset.MetadataJSON) > 0 && json.Unmarshal(asset.MetadataJSON, &metadata) != nil {
			return nil, fmt.Errorf("asset has invalid identity metadata")
		}
		if len(metadata.LockedTraits) > 32 {
			return nil, fmt.Errorf("asset has too many locked traits")
		}
		for key, value := range metadata.LockedTraits {
			if strings.TrimSpace(key) == "" || len(key) > 64 || strings.TrimSpace(value) == "" || len(value) > 500 {
				return nil, fmt.Errorf("asset has an invalid locked trait")
			}
		}
		item := directorAsset{ID: asset.ID, Name: asset.Name, Type: asset.Type, Description: asset.Description, CharacterID: asset.CharacterID, Tags: asset.TagsJSON, LockedTraits: metadata.LockedTraits, UpdatedAt: asset.UpdatedAt}
		path, err := s.storagePath(asset.FilePath)
		if err == nil {
			file, openErr := os.Open(path)
			if openErr == nil {
				hash := sha256.New()
				n, readErr := io.Copy(hash, io.LimitReader(file, (64<<20)+1))
				file.Close()
				if readErr == nil && n > 0 && n <= 64<<20 {
					item.Available = true
					item.ContentSHA256 = hex.EncodeToString(hash.Sum(nil))
				}
			}
		}
		if !item.Available {
			result.Warnings = append(result.Warnings, "Reference asset "+asset.ID+" is missing, unreadable, empty or larger than 64 MiB")
		}
		result.Assets = append(result.Assets, item)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(raw)
	result.Revision = hex.EncodeToString(hash[:])
	// Retain explicit selection separately from expanded character references. It is not part of the resolved identity digest.
	result.Selection = &directorSelection{CharacterIDs: append([]string{}, selection.CharacterIDs...), AssetIDs: append([]string{}, selection.AssetIDs...)}
	return result, nil
}
