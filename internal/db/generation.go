package db

import (
	"encoding/json"
	"fmt"
	"time"
	"videoflow-go/internal/models"
)

func (d *DB) SaveScriptScenes(script *models.Script, scenes []models.Scene) error {
	return d.SaveStorySnapshot(script, scenes, nil)
}

// SaveStorySnapshot commits the planned cast and story as one unit.
func (d *DB) SaveStorySnapshot(script *models.Script, scenes []models.Scene, characters []models.Character) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i := range characters {
		if script.ProjectID == nil || characters[i].ProjectID == nil || *characters[i].ProjectID != *script.ProjectID {
			return fmt.Errorf("story character project mismatch")
		}
		if err = insertCharacter(tx, &characters[i]); err != nil {
			return err
		}
	}
	if err = insertScript(tx, script); err != nil {
		return err
	}
	for i := range scenes {
		scenes[i].ScriptID = &script.ID
		scenes[i].ProjectID = script.ProjectID
		var cast []string
		if len(scenes[i].CharacterIDsJSON) > 0 {
			if err = json.Unmarshal(scenes[i].CharacterIDsJSON, &cast); err != nil {
				return err
			}
		}
		for _, id := range cast {
			var owner string
			if err = tx.QueryRow(`SELECT project_id FROM characters WHERE id=?`, id).Scan(&owner); err != nil {
				return err
			}
			if script.ProjectID == nil || owner != *script.ProjectID {
				return fmt.Errorf("story cast belongs to another project")
			}
		}
		if err = insertScene(tx, &scenes[i]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ReplaceSceneShots saves one complete approved generation snapshot, not a partial append.
func (d *DB) ReplaceSceneShots(scene *models.Scene, shots []models.Shot) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var project string
	var updated time.Time
	if err = tx.QueryRow(`SELECT project_id,updated_at FROM scenes WHERE id=?`, scene.ID).Scan(&project, &updated); err != nil {
		return err
	}
	if !updated.Equal(scene.UpdatedAt) {
		return fmt.Errorf("scene changed during generation; approved work was not replaced")
	}
	if scene.ProjectID == nil || project != *scene.ProjectID {
		return fmt.Errorf("scene project mismatch")
	}
	var ids []string
	if err = json.Unmarshal(scene.CharacterIDsJSON, &ids); err != nil {
		return err
	}
	for _, id := range ids {
		var owner string
		if err = tx.QueryRow(`SELECT project_id FROM characters WHERE id=?`, id).Scan(&owner); err != nil {
			return err
		}
		if owner != project {
			return fmt.Errorf("cast belongs to another project")
		}
	}
	if _, err = tx.Exec(`DELETE FROM shots WHERE scene_id=?`, scene.ID); err != nil {
		return err
	}
	for i := range shots {
		shots[i].SceneID = scene.ID
		if err = insertShot(tx, &shots[i]); err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	if _, err = tx.Exec(`UPDATE scenes SET character_ids_json=?,scene_json=?,updated_at=? WHERE id=?`, string(scene.CharacterIDsJSON), string(scene.SceneJSON), now, scene.ID); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	scene.UpdatedAt = now
	return nil
}

func (d *DB) SaveLocalOutput(job *models.RenderJob, output *models.RenderOutput) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = insertRenderJob(tx, job); err != nil {
		return err
	}
	output.RenderJobID = job.ID
	if err = insertRenderOutput(tx, output); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) AppendCharacterReferences(id, project string, assetIDs []string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owner string
	if err = tx.QueryRow(`SELECT project_id FROM characters WHERE id=?`, id).Scan(&owner); err != nil {
		return err
	}
	if owner != project {
		return fmt.Errorf("character project mismatch")
	}
	for _, assetID := range assetIDs {
		var assetProject string
		if err = tx.QueryRow(`SELECT project_id FROM assets WHERE id=?`, assetID).Scan(&assetProject); err != nil {
			return err
		}
		if assetProject != project {
			return fmt.Errorf("reference project mismatch")
		}
		if _, err = tx.Exec(`UPDATE characters SET reference_asset_ids_json=json_insert(reference_asset_ids_json,'$[#]',?),updated_at=? WHERE id=?`, assetID, time.Now().UTC(), id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ClaimStoryContinuation fences concurrent/replayed continuation clicks before
// any paid work; the exact saved snapshot must still be waiting for review.
func (d *DB) ClaimStoryContinuation(id, project string, expected json.RawMessage) (bool, error) {
	result, err := d.conn.Exec(`UPDATE ops SET status='running',error=NULL,updated_at=? WHERE id=? AND project_id=? AND kind='video_generation' AND status='succeeded' AND result_json=?`, time.Now().UTC(), id, project, string(expected))
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// ReplaceUnapprovedCharacterReferences keeps approved active inputs and retires
// older candidates only after a new image is saved. Asset history is untouched.
func (d *DB) ReplaceUnapprovedCharacterReferences(id, project string, created []string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw string
	if err = tx.QueryRow(`SELECT reference_asset_ids_json FROM characters WHERE id=? AND project_id=?`, id, project).Scan(&raw); err != nil {
		return err
	}
	var current []string
	if err = json.Unmarshal([]byte(raw), &current); err != nil {
		return err
	}
	next := []string{}
	seen := map[string]bool{}
	for _, ref := range current {
		var meta string
		if err = tx.QueryRow(`SELECT metadata_json FROM assets WHERE id=? AND project_id=?`, ref, project).Scan(&meta); err != nil {
			return err
		}
		var state struct {
			Review string `json:"consistency_review"`
		}
		_ = json.Unmarshal([]byte(meta), &state) // unreadable metadata has no review verdict
		if (state.Review == "" || state.Review == "approved") && !seen[ref] {
			next = append(next, ref)
			seen[ref] = true
		}
	}
	for _, ref := range created {
		var owner, kind string
		if err = tx.QueryRow(`SELECT character_id,type FROM assets WHERE id=? AND project_id=?`, ref, project).Scan(&owner, &kind); err != nil {
			return err
		}
		if owner != id || kind != "character_reference" {
			return fmt.Errorf("candidate is not a reference for this character")
		}
		if !seen[ref] {
			next = append(next, ref)
			seen[ref] = true
		}
	}
	rawNext, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE characters SET reference_asset_ids_json=?,updated_at=? WHERE id=? AND project_id=?`, string(rawNext), time.Now().UTC(), id, project); err != nil {
		return err
	}
	return tx.Commit()
}

// ExcludeStoryAsset atomically updates the paused operation and its scene bindings.
// The library asset is retained. Exact result matching prevents racing continuation.
func (d *DB) ExcludeStoryAsset(op, project, asset string, scenes []string, expected, next []byte) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE ops SET result_json=? WHERE id=? AND project_id=? AND kind='video_generation' AND status='succeeded' AND result_json=?`, string(next), op, project, string(expected))
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return fmt.Errorf("story changed; refresh before removing a reference")
	}
	remove := func(ids []string) []string {
		result := []string{}
		for _, id := range ids {
			if id != asset {
				result = append(result, id)
			}
		}
		return result
	}
	for _, id := range scenes {
		var sceneRaw, assetRaw []byte
		if err = tx.QueryRow(`SELECT scene_json,asset_ids_json FROM scenes WHERE id=? AND project_id=?`, id, project).Scan(&sceneRaw, &assetRaw); err != nil {
			return err
		}
		var metadata map[string]json.RawMessage
		if err = json.Unmarshal(sceneRaw, &metadata); err != nil {
			return err
		}
		if metadata == nil {
			metadata = map[string]json.RawMessage{}
		}
		refs, all := models.IDList(metadata["story_asset_ids"]), models.IDList(assetRaw)
		metadata["story_asset_ids"], _ = json.Marshal(remove(refs))
		sceneRaw, _ = json.Marshal(metadata)
		assetRaw, _ = json.Marshal(remove(all))
		if _, err = tx.Exec(`UPDATE scenes SET scene_json=?,asset_ids_json=?,updated_at=? WHERE id=? AND project_id=?`, string(sceneRaw), string(assetRaw), time.Now().UTC(), id, project); err != nil {
			return err
		}
	}
	return tx.Commit()
}
