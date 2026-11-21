package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// VoiceDocument is a revisioned record kept per project, such as the saved voice
// of a character. Saving needs the revision last read, so two edits never collide.
type VoiceDocument struct {
	ID        string          `json:"id"`
	ProjectID string          `json:"project_id"`
	Kind      string          `json:"kind"`
	Revision  int             `json:"revision"`
	Data      json.RawMessage `json:"data"`
}

// EnsureVoiceWorkflow creates the tables. voice_tasks is no longer written or read;
// it stays so databases from earlier versions keep their rows.
func (d *DB) EnsureVoiceWorkflow() error {
	_, err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS voice_documents(project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,kind TEXT NOT NULL,id TEXT NOT NULL,revision INTEGER NOT NULL,data TEXT NOT NULL,PRIMARY KEY(project_id,kind,id));
 CREATE TABLE IF NOT EXISTS voice_tasks(id TEXT PRIMARY KEY,project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,fingerprint TEXT NOT NULL,kind TEXT NOT NULL,model TEXT NOT NULL,endpoint TEXT NOT NULL,status TEXT NOT NULL,upstream_id TEXT NOT NULL DEFAULT '',input TEXT NOT NULL,bindings TEXT NOT NULL,result TEXT NOT NULL DEFAULT 'null',error TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,UNIQUE(project_id,fingerprint));`)
	return err
}
func (d *DB) GetVoiceDocument(project, kind, id string) (*VoiceDocument, error) {
	v := &VoiceDocument{ID: id, ProjectID: project, Kind: kind}
	var data string
	err := d.conn.QueryRow(`SELECT revision,data FROM voice_documents WHERE project_id=? AND kind=? AND id=?`, project, kind, id).Scan(&v.Revision, &data)
	v.Data = json.RawMessage(data)
	return v, err
}
func (d *DB) ListVoiceDocuments(project, kind string) ([]VoiceDocument, error) {
	rows, err := d.conn.Query(`SELECT id,revision,data FROM voice_documents WHERE project_id=? AND kind=? ORDER BY id`, project, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VoiceDocument{}
	for rows.Next() {
		v := VoiceDocument{ProjectID: project, Kind: kind}
		var raw string
		if err = rows.Scan(&v.ID, &v.Revision, &raw); err != nil {
			return nil, err
		}
		v.Data = json.RawMessage(raw)
		out = append(out, v)
	}
	return out, rows.Err()
}
func (d *DB) SaveVoiceDocument(project, kind, id string, revision int, data json.RawMessage) (*VoiceDocument, error) {
	if !json.Valid(data) {
		return nil, fmt.Errorf("invalid voice document")
	}
	var res sql.Result
	var err error
	if revision == 0 {
		res, err = d.conn.Exec(`INSERT OR IGNORE INTO voice_documents(project_id,kind,id,revision,data)VALUES(?,?,?,1,?)`, project, kind, id, string(data))
	} else {
		res, err = d.conn.Exec(`UPDATE voice_documents SET revision=revision+1,data=? WHERE project_id=? AND kind=? AND id=? AND revision=?`, string(data), project, kind, id, revision)
	}
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return nil, fmt.Errorf("voice document changed; reload before saving")
	}
	return d.GetVoiceDocument(project, kind, id)
}
