package db

import (
	"encoding/json"
)

// CompareDirectorSelection protects concurrent context edits and running sessions.
func (d *DB) CompareDirectorSelection(session, project string, old, next json.RawMessage) (bool, error) {
	result, err := d.conn.Exec(`UPDATE director_contexts SET selection=? WHERE session_id=? AND selection=? AND EXISTS(SELECT 1 FROM director_sessions WHERE id=? AND project_id=? AND status!='running')`, string(next), session, string(old), session, project)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
