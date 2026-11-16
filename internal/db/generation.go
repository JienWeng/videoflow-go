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

// SaveOperationJobs records the complete batch before any worker can submit paid work.
func (d *DB) SaveOperationJobs(opID string, jobs []models.RenderJob, result map[string]any) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ids := []string{}
	for i := range jobs {
		if err = insertRenderJob(tx, &jobs[i]); err != nil {
			return err
		}
		ids = append(ids, jobs[i].ID)
	}
	result["render_job_ids"] = ids
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	res, err := tx.Exec(`UPDATE ops SET status='succeeded',result_json=?,updated_at=? WHERE id=? AND status='running'`, string(raw), time.Now().UTC(), opID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("operation is no longer running")
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
