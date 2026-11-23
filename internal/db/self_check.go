package db

import (
	"encoding/json"
	"fmt"
)

// SetAssetSelfCheck stores a picture check result under metadata_json.self_check. It
// leaves updated_at and every other metadata key alone: a check is a note about the
// picture, not an edit, so it never makes a storyboard look outdated or a review stale.
func (d *DB) SetAssetSelfCheck(project, id string, record json.RawMessage) error {
	result, err := d.conn.Exec(`UPDATE assets SET metadata_json=json_set(metadata_json,'$.self_check',json(?)) WHERE id=? AND project_id=? AND json_valid(metadata_json) AND json_type(metadata_json)='object'`, string(record), id, project)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return fmt.Errorf("picture not found in this project, or its notes need repair")
	}
	return nil
}

// SetRenderOutputSelfCheck stores a clip check result under qa_json.self_check, the
// same way: other clip notes and updated_at are kept.
func (d *DB) SetRenderOutputSelfCheck(id string, record json.RawMessage) error {
	result, err := d.conn.Exec(`UPDATE render_outputs SET qa_json=json_set(CASE WHEN qa_json IS NULL OR qa_json='' THEN '{}' ELSE qa_json END,'$.self_check',json(?)) WHERE id=? AND (qa_json IS NULL OR qa_json='' OR (json_valid(qa_json) AND json_type(qa_json)='object'))`, string(record), id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return fmt.Errorf("clip not found, or its notes need repair")
	}
	return nil
}
