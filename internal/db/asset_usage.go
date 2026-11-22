package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
	"videoflow-go/internal/models"
)

type AssetUse struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

const assetUsageSQL = `
 SELECT 'scene',id,title FROM scenes WHERE project_id=? AND (EXISTS(SELECT 1 FROM json_tree(CASE WHEN json_valid(asset_ids_json) THEN asset_ids_json ELSE '[]' END) WHERE value=?) OR EXISTS(SELECT 1 FROM json_tree(CASE WHEN json_valid(scene_json) THEN scene_json ELSE '{}' END) WHERE value=?))
 UNION SELECT 'character',id,name FROM characters WHERE project_id=? AND EXISTS(SELECT 1 FROM json_tree(reference_asset_ids_json) WHERE value=?)
 UNION SELECT 'shot',sh.id,sc.title FROM shots sh JOIN scenes sc ON sc.id=sh.scene_id WHERE sc.project_id=? AND (EXISTS(SELECT 1 FROM json_tree(sh.asset_ids_json) WHERE value=?) OR EXISTS(SELECT 1 FROM json_tree(sh.shot_json) WHERE value=?))
 UNION SELECT 'render',id,COALESCE(scene_id,'Video render') FROM render_jobs WHERE project_id=? AND EXISTS(SELECT 1 FROM json_tree(request_json) WHERE value=?)
 UNION SELECT 'version',id,name FROM assets WHERE project_id=? AND id!=? AND EXISTS(SELECT 1 FROM json_tree(metadata_json) WHERE key IN ('source_asset_id','variant_group_id') AND value=?)
 UNION SELECT 'session',ds.id,ds.title FROM director_contexts dc JOIN director_sessions ds ON ds.id=dc.session_id WHERE ds.project_id=? AND EXISTS(SELECT 1 FROM json_tree(dc.selection) WHERE value=?)`

func assetUses(query interface {
	Query(string, ...any) (*sql.Rows, error)
}, project, id string) ([]AssetUse, error) {
	rows, err := query.Query(assetUsageSQL, project, id, id, project, id, project, id, id, project, id, project, id, id, project, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []AssetUse{}
	for rows.Next() {
		var use AssetUse
		if err = rows.Scan(&use.Kind, &use.ID, &use.Name); err != nil {
			return nil, err
		}
		result = append(result, use)
	}
	return result, rows.Err()
}
func (d *DB) AssetUses(project, id string) ([]AssetUse, error) { return assetUses(d.conn, project, id) }

// DeleteUnusedAsset keeps shared/frozen references valid and checks ownership in the same transaction.
func (d *DB) DeleteUnusedAsset(project, id string) ([]AssetUse, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var owned string
	if err = tx.QueryRow(`SELECT id FROM assets WHERE id=? AND project_id=?`, id, project).Scan(&owned); err != nil {
		return nil, err
	}
	uses, err := assetUses(tx, project, id)
	if err != nil {
		return nil, err
	}
	if len(uses) > 0 {
		return uses, nil
	}
	if _, err = tx.Exec(`DELETE FROM assets WHERE id=? AND project_id=?`, id, project); err != nil {
		return nil, err
	}
	return uses, tx.Commit()
}

// SetSceneReference changes both the canonical conditioning list and the graph association atomically.
func (d *DB) SetSceneReference(project, sceneID, assetID string, attach bool, replace string, expected time.Time) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw, allRaw []byte
	var updated time.Time
	if err = tx.QueryRow(`SELECT scene_json,asset_ids_json,updated_at FROM scenes WHERE id=? AND project_id=?`, sceneID, project).Scan(&raw, &allRaw, &updated); err != nil {
		return err
	}
	if !updated.Equal(expected) {
		return fmt.Errorf("scene changed; refresh references before changing them")
	}
	if attach {
		var owned string
		if err = tx.QueryRow(`SELECT id FROM assets WHERE id=? AND project_id=? AND type IN ('prop','location')`, assetID, project).Scan(&owned); err != nil {
			return fmt.Errorf("asset is no longer available in this project")
		}
	}
	var metadata map[string]json.RawMessage
	if err = json.Unmarshal(raw, &metadata); err != nil {
		return err
	}
	if metadata == nil {
		metadata = map[string]json.RawMessage{}
	}
	refs, all := models.IDList(metadata["story_asset_ids"]), models.IDList(allRaw)
	update := func(ids []string) []string {
		result := []string{}
		seen := false
		for _, id := range ids {
			if id == assetID {
				if !attach || seen {
					continue
				}
				seen = true
			}
			result = append(result, id)
		}
		if attach && !seen {
			result = append(result, assetID)
		}
		return result
	}
	if replace != "" {
		found := false
		for _, id := range append(refs, all...) {
			if id == replace {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("the version being replaced is no longer selected")
		}
	}
	removed := assetID
	if attach {
		removed = replace
	}
	removeID := func(ids []string) []string {
		result := []string{}
		for _, id := range ids {
			if id != removed {
				result = append(result, id)
			}
		}
		return result
	}
	if replace != "" {
		refs = removeID(refs)
		all = removeID(all)
	}
	if removed != "" {
		var brief map[string]json.RawMessage
		if json.Unmarshal(metadata["generation_brief"], &brief) == nil && brief != nil {
			var oldRefs []string
			if json.Unmarshal(brief["reference_asset_ids"], &oldRefs) == nil {
				brief["reference_asset_ids"], _ = json.Marshal(removeID(oldRefs))
				metadata["generation_brief"], _ = json.Marshal(brief)
			}
		}
	}
	metadata["story_asset_ids"], _ = json.Marshal(update(refs))
	raw, _ = json.Marshal(metadata)
	allRaw, _ = json.Marshal(update(all))
	if _, err = tx.Exec(`UPDATE scenes SET scene_json=?,asset_ids_json=?,updated_at=? WHERE id=? AND project_id=?`, string(raw), string(allRaw), time.Now().UTC(), sceneID, project); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) UpdateAssetDetails(asset *models.Asset, expected time.Time) error {
	result, err := d.conn.Exec(`UPDATE assets SET name=?,description=?,metadata_json=?,updated_at=? WHERE id=? AND project_id=? AND updated_at=?`, asset.Name, asset.Description, string(asset.MetadataJSON), time.Now().UTC(), asset.ID, asset.ProjectID, expected)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return fmt.Errorf("asset changed; reload before saving")
	}
	return nil
}
