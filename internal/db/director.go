package db

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"time"
)

type DirectorSession struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Engine    string `json:"engine"`
	NativeID  string `json:"native_id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Selected  bool   `json:"selected"`
	CreatedAt string `json:"created_at"`
}
type DirectorTurn struct {
	ID          string          `json:"id"`
	SessionID   string          `json:"session_id"`
	Kind        string          `json:"kind"`
	Status      string          `json:"status"`
	RequestJSON json.RawMessage `json:"request_json"`
	ResultJSON  json.RawMessage `json:"result_json"`
	Error       string          `json:"error"`
	CreatedAt   string          `json:"created_at"`
}

func (d *DB) ensureDirectorWorkflow() error {
	_, err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS director_sessions(id TEXT PRIMARY KEY,project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,engine TEXT NOT NULL,native_id TEXT NOT NULL DEFAULT '',title TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'ready',selected INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL);
 CREATE UNIQUE INDEX IF NOT EXISTS director_selected ON director_sessions(project_id) WHERE selected=1;
 CREATE TABLE IF NOT EXISTS director_turns(id TEXT PRIMARY KEY,session_id TEXT NOT NULL REFERENCES director_sessions(id) ON DELETE CASCADE,request_id TEXT NOT NULL,kind TEXT NOT NULL,status TEXT NOT NULL,request_json TEXT NOT NULL,result_json TEXT NOT NULL DEFAULT 'null',error TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL,UNIQUE(session_id,request_id));
 CREATE TABLE IF NOT EXISTS director_contexts(session_id TEXT PRIMARY KEY REFERENCES director_sessions(id) ON DELETE CASCADE,selection TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS director_proposals(id TEXT PRIMARY KEY,session_id TEXT NOT NULL REFERENCES director_sessions(id) ON DELETE CASCADE,project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,revision TEXT NOT NULL,selection TEXT NOT NULL);
 UPDATE director_sessions SET status='review_required' WHERE status='running';
 UPDATE director_turns SET status='interrupted',error='Backend restarted during this turn. Review saved native session before continuing; no automatic replay.' WHERE status='running';`)
	return err
}
func (d *DB) CreateDirectorSession(project, engine, title string) (*DirectorSession, error) {
	if engine != "codex" && engine != "claude" {
		return nil, fmt.Errorf("select codex or claude")
	}
	if title == "" {
		title = "New production session"
	}
	if len(title) > 120 {
		return nil, fmt.Errorf("session title exceeds 120 characters")
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE director_sessions SET selected=0 WHERE project_id=?`, project); err != nil {
		return nil, err
	}
	s := &DirectorSession{ID: uuid.NewString(), ProjectID: project, Engine: engine, Title: title, Status: "ready", Selected: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	_, err = tx.Exec(`INSERT INTO director_sessions(id,project_id,engine,title,selected,created_at) VALUES(?,?,?,?,1,?)`, s.ID, project, engine, title, s.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return s, nil
}
func (d *DB) GetDirectorSession(id, project string) (*DirectorSession, error) {
	s := &DirectorSession{}
	err := d.conn.QueryRow(`SELECT id,project_id,engine,native_id,title,status,selected,created_at FROM director_sessions WHERE id=? AND project_id=?`, id, project).Scan(&s.ID, &s.ProjectID, &s.Engine, &s.NativeID, &s.Title, &s.Status, &s.Selected, &s.CreatedAt)
	return s, err
}
func (d *DB) ListDirectorSessions(project string) ([]DirectorSession, error) {
	rows, err := d.conn.Query(`SELECT id,project_id,engine,native_id,title,status,selected,created_at FROM director_sessions WHERE project_id=? ORDER BY created_at DESC`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DirectorSession{}
	for rows.Next() {
		var s DirectorSession
		if err = rows.Scan(&s.ID, &s.ProjectID, &s.Engine, &s.NativeID, &s.Title, &s.Status, &s.Selected, &s.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
func (d *DB) SelectDirectorSession(id, project string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRow(`SELECT count(*) FROM director_sessions WHERE id=? AND project_id=?`, id, project).Scan(&exists); err != nil {
		return err
	}
	if exists != 1 {
		return fmt.Errorf("session not found in project")
	}
	if _, err = tx.Exec(`UPDATE director_sessions SET selected=0 WHERE project_id=?`, project); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE director_sessions SET selected=1 WHERE id=? AND project_id=?`, id, project); err != nil {
		return err
	}
	return tx.Commit()
}
func (d *DB) AcknowledgeDirectorSession(id, project string) error {
	result, err := d.conn.Exec(`UPDATE director_sessions SET status='ready' WHERE id=? AND project_id=? AND status='review_required'`, id, project)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return fmt.Errorf("session is not awaiting review")
	}
	return nil
}
func (d *DB) SetDirectorNativeID(id, native string) error {
	result, err := d.conn.Exec(`UPDATE director_sessions SET native_id=? WHERE id=? AND (native_id='' OR native_id=?)`, native, id, native)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return fmt.Errorf("native session identity mismatch")
	}
	return nil
}
func (d *DB) BeginDirectorTurn(session, project, request, kind string, raw json.RawMessage) (*DirectorTurn, error) {
	if request == "" || len(request) > 160 {
		return nil, fmt.Errorf("request ID is required")
	}
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE director_sessions SET status='running' WHERE id=? AND project_id=? AND status='ready'`, session, project)
	if err != nil {
		return nil, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return nil, fmt.Errorf("session busy or requires review; open Local sessions")
	}
	turn := &DirectorTurn{ID: uuid.NewString(), SessionID: session, Kind: kind, Status: "running", RequestJSON: raw, ResultJSON: json.RawMessage(`null`), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if _, err = tx.Exec(`INSERT INTO director_turns(id,session_id,request_id,kind,status,request_json,created_at) VALUES(?,?,?,?,?,?,?)`, turn.ID, session, request, kind, turn.Status, string(raw), turn.CreatedAt); err != nil {
		return nil, fmt.Errorf("request already recorded or unavailable: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return turn, nil
}
func (d *DB) FinishDirectorTurn(id, status string, result json.RawMessage, message string) error {
	if status != "succeeded" && status != "failed" && status != "interrupted" {
		return fmt.Errorf("invalid turn status")
	}
	if len(result) == 0 {
		result = json.RawMessage(`null`)
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	updated, err := tx.Exec(`UPDATE director_turns SET status=?,result_json=?,error=? WHERE id=? AND status='running'`, status, string(result), message, id)
	if err != nil {
		return err
	}
	n, _ := updated.RowsAffected()
	if n != 1 {
		return fmt.Errorf("turn is no longer running")
	}
	sessionStatus := "ready"
	if status != "succeeded" {
		sessionStatus = "review_required"
	}
	if _, err = tx.Exec(`UPDATE director_sessions SET status=? WHERE id=(SELECT session_id FROM director_turns WHERE id=?)`, sessionStatus, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (d *DB) ListDirectorTurns(session, project string) ([]DirectorTurn, error) {
	if _, err := d.GetDirectorSession(session, project); err != nil {
		return nil, err
	}
	rows, err := d.conn.Query(`SELECT id,session_id,kind,status,request_json,result_json,error,created_at FROM director_turns WHERE session_id=? ORDER BY created_at ASC LIMIT 200`, session)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DirectorTurn{}
	for rows.Next() {
		var t DirectorTurn
		var request, reply string
		if err = rows.Scan(&t.ID, &t.SessionID, &t.Kind, &t.Status, &request, &reply, &t.Error, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.RequestJSON = json.RawMessage(request)
		t.ResultJSON = json.RawMessage(reply)
		result = append(result, t)
	}
	return result, rows.Err()
}
func (d *DB) GetDirectorSelection(session string) (json.RawMessage, error) {
	var raw string
	err := d.conn.QueryRow(`SELECT selection FROM director_contexts WHERE session_id=?`, session).Scan(&raw)
	if err != nil {
		return json.RawMessage(`{}`), nil
	}
	return json.RawMessage(raw), nil
}
func (d *DB) SetDirectorSelection(session, project string, selection json.RawMessage) error {
	result, err := d.conn.Exec(`INSERT INTO director_contexts(session_id,selection) SELECT id,? FROM director_sessions WHERE id=? AND project_id=? AND status!='running' ON CONFLICT(session_id) DO UPDATE SET selection=excluded.selection`, string(selection), session, project)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return fmt.Errorf("session busy or not found")
	}
	return nil
}

type DirectorProposal struct {
	ID, SessionID, ProjectID, Revision string
	Selection                          json.RawMessage
}

func (d *DB) CreateDirectorProposal(session, project, revision string, selection json.RawMessage) (string, error) {
	id := uuid.NewString()
	_, err := d.conn.Exec(`INSERT INTO director_proposals(id,session_id,project_id,revision,selection) VALUES(?,?,?,?,?)`, id, session, project, revision, string(selection))
	return id, err
}
func (d *DB) GetDirectorProposal(id, project string) (*DirectorProposal, error) {
	p := &DirectorProposal{}
	var raw string
	err := d.conn.QueryRow(`SELECT id,session_id,project_id,revision,selection FROM director_proposals WHERE id=? AND project_id=?`, id, project).Scan(&p.ID, &p.SessionID, &p.ProjectID, &p.Revision, &raw)
	p.Selection = json.RawMessage(raw)
	return p, err
}
