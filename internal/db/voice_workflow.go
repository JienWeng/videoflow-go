package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// VoiceDocument revisions bind approvals to saved profiles and spoken lines.
type VoiceDocument struct {
	ID        string          `json:"id"`
	ProjectID string          `json:"project_id"`
	Kind      string          `json:"kind"`
	Revision  int             `json:"revision"`
	Data      json.RawMessage `json:"data"`
}
type VoiceBinding struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision int    `json:"revision,omitempty"`
	Hash     string `json:"hash,omitempty"`
}
type VoiceTask struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	Fingerprint string          `json:"fingerprint"`
	Kind        string          `json:"kind"`
	Model       string          `json:"model"`
	Endpoint    string          `json:"endpoint"`
	Status      string          `json:"status"`
	UpstreamID  string          `json:"upstream_id"`
	Preview     json.RawMessage `json:"preview"`
	Input       json.RawMessage `json:"-"` // May contain approved sample bytes; never send them back in task listings.
	Bindings    []VoiceBinding  `json:"bindings"`
	Result      json.RawMessage `json:"result,omitempty"`
	Error       string          `json:"error,omitempty"`
	CreatedAt   string          `json:"created_at"`
}

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
func (d *DB) CreateVoiceTask(t VoiceTask) (*VoiceTask, error) {
	bindings, _ := json.Marshal(t.Bindings)
	_, err := d.conn.Exec(`INSERT OR IGNORE INTO voice_tasks(id,project_id,fingerprint,kind,model,endpoint,status,input,bindings)VALUES(?,?,?,?,?,?,?,?,?)`, t.ID, t.ProjectID, t.Fingerprint, t.Kind, t.Model, t.Endpoint, "draft", string(t.Input), string(bindings))
	if err != nil {
		return nil, err
	}
	var id string
	if err = d.conn.QueryRow(`SELECT id FROM voice_tasks WHERE project_id=? AND fingerprint=?`, t.ProjectID, t.Fingerprint).Scan(&id); err != nil {
		return nil, err
	}
	return d.GetVoiceTask(t.ProjectID, id)
}

const voiceTaskColumns = `id,project_id,fingerprint,kind,model,endpoint,status,upstream_id,input,bindings,result,error,created_at`

func scanVoiceTask(row interface{ Scan(...any) error }) (*VoiceTask, error) {
	t := &VoiceTask{}
	var input, bindings, result string
	err := row.Scan(&t.ID, &t.ProjectID, &t.Fingerprint, &t.Kind, &t.Model, &t.Endpoint, &t.Status, &t.UpstreamID, &input, &bindings, &result, &t.Error, &t.CreatedAt)
	t.Input = json.RawMessage(input)
	var full map[string]json.RawMessage
	json.Unmarshal(t.Input, &full)
	preview := map[string]json.RawMessage{}
	for _, key := range []string{"text", "prompt", "duration", "aspect_ratio", "voice_setting", "sync_mode"} {
		if value, ok := full[key]; ok {
			preview[key] = value
		}
	}
	t.Preview, _ = json.Marshal(preview)
	t.Result = json.RawMessage(result)
	if err == nil {
		err = json.Unmarshal([]byte(bindings), &t.Bindings)
	}
	return t, err
}
func (d *DB) GetVoiceTask(project, id string) (*VoiceTask, error) {
	return scanVoiceTask(d.conn.QueryRow(`SELECT `+voiceTaskColumns+` FROM voice_tasks WHERE project_id=? AND id=?`, project, id))
}
func (d *DB) ListVoiceTasks(project string) ([]VoiceTask, error) {
	rows, err := d.conn.Query(`SELECT `+voiceTaskColumns+` FROM voice_tasks WHERE project_id=? ORDER BY created_at DESC,id`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VoiceTask{}
	for rows.Next() {
		t, err := scanVoiceTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// The durable fence must succeed BEFORE the first request byte can leave the app.
// A process lost in submitting is intentionally not reset to draft on startup.
func (d *DB) ClaimVoiceTask(project, id string) error {
	res, err := d.conn.Exec(`UPDATE voice_tasks SET status='submitting' WHERE project_id=? AND id=? AND status='draft'`, project, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return fmt.Errorf("task already submitted or uncertain; reconcile, never resubmit")
	}
	return nil
}
func (d *DB) UpdateVoiceTask(t *VoiceTask) error {
	result := t.Result
	if len(result) == 0 {
		result = json.RawMessage(`null`)
	}
	_, err := d.conn.Exec(`UPDATE voice_tasks SET status=?,upstream_id=?,result=?,error=? WHERE project_id=? AND id=?`, t.Status, t.UpstreamID, string(result), t.Error, t.ProjectID, t.ID)
	return err
}
