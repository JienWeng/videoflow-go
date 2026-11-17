package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SaveSceneVoices commits the complete setup only if the scene and every source
// profile revision still match the displayed scene setup. Never partially saves.
func (d *DB) SaveSceneVoices(project, scene string, expected time.Time, docs []VoiceDocument) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var stamp time.Time
	if err = tx.QueryRow(`SELECT updated_at FROM scenes WHERE id=? AND project_id=?`, scene, project).Scan(&stamp); err != nil || !stamp.Equal(expected) {
		return fmt.Errorf("scene changed; recheck setup before saving")
	}
	seen := map[string]bool{}
	for _, doc := range docs {
		if seen[doc.ID] || !json.Valid(doc.Data) {
			return fmt.Errorf("duplicate or invalid voice profile")
		}
		seen[doc.ID] = true
		var owned string
		if err = tx.QueryRow(`SELECT id FROM characters WHERE id=? AND project_id=?`, doc.ID, project).Scan(&owned); err != nil {
			return fmt.Errorf("speaker is no longer in this project")
		}
		var res sql.Result
		if doc.Revision == 0 {
			res, err = tx.Exec(`INSERT OR IGNORE INTO voice_documents(project_id,kind,id,revision,data) VALUES(?,'profile',?,1,?)`, project, doc.ID, string(doc.Data))
		} else {
			res, err = tx.Exec(`UPDATE voice_documents SET revision=revision+1,data=? WHERE project_id=? AND kind='profile' AND id=? AND revision=?`, string(doc.Data), project, doc.ID, doc.Revision)
		}
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return fmt.Errorf("a speaker voice changed; recheck setup before saving")
		}
	}
	return tx.Commit()
}
