package db

import (
	"encoding/json"
	"fmt"
	"time"
)

// UpdateAssetReview compares the row revision atomically, preserving concurrent
// identity edits rather than applying an approval to a different asset version.
func (d *DB) UpdateAssetReview(project, id string, expected time.Time, meta json.RawMessage) error {
	result, e := d.conn.Exec(`UPDATE assets SET metadata_json=?,updated_at=? WHERE id=? AND project_id=? AND updated_at=?`, string(meta), time.Now().UTC(), id, project, expected)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return fmt.Errorf("asset changed; reopen it before reviewing")
	}
	return nil
}

// UpdateAssetRecognition changes only recognition-owned fields, preserving
// identity/review metadata edited while a remote vision call was in flight.
func (d *DB) UpdateAssetRecognition(project, id, description string, tags json.RawMessage) error {
	result, err := d.conn.Exec(`UPDATE assets SET description=?,tags_json=?,updated_at=? WHERE id=? AND project_id=?`, description, string(tags), time.Now().UTC(), id, project)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("asset not found in project")
	}
	return nil
}
