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
	return d.SetRenderOutputNote(id, "self_check", record)
}

// renderOutputNotes are the qa_json keys written one at a time, so a note written by
// one step (the speech check, caption timing) never erases another's.
var renderOutputNotes = map[string]bool{"self_check": true, "audio_check": true, "speech_timing": true, "approval": true}

// SetRenderOutputNote stores one note under qa_json.<key>, keeping every other note
// and updated_at.
func (d *DB) SetRenderOutputNote(id, key string, record json.RawMessage) error {
	if !renderOutputNotes[key] {
		return fmt.Errorf("unknown clip note %q", key)
	}
	result, err := d.conn.Exec(`UPDATE render_outputs SET qa_json=json_set(CASE WHEN qa_json IS NULL OR qa_json='' THEN '{}' ELSE qa_json END,'$.`+key+`',json(?)) WHERE id=? AND (qa_json IS NULL OR qa_json='' OR (json_valid(qa_json) AND json_type(qa_json)='object'))`, string(record), id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return fmt.Errorf("clip not found, or its notes need repair")
	}
	return nil
}
