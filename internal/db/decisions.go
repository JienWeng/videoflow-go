package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ProjectDecision is one entry of a project's decision log: a choice made for the
// project (its language, format, a character's voice, a rejected proposal) that the
// assistant reads on every turn and does not ask about again. Key is unique per
// project; saving a key again replaces its value.
type ProjectDecision struct {
	ProjectID string    `json:"project_id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Source    string    `json:"source"` // user or assistant
	CreatedAt time.Time `json:"created_at"`
}

// Who made a decision.
const (
	DecisionByUser      = "user"
	DecisionByAssistant = "assistant"
)

// ErrDecisionLogFull is returned when a project already keeps MaxProjectDecisions.
var ErrDecisionLogFull = errors.New("this project already keeps the most decisions it can; delete some first")

// decisionTime is how created_at is stored: fixed width, so text order is time order.
const decisionTime = "2006-01-02T15:04:05.000000000Z"

// MaxProjectDecisions caps one project's log so every chat turn can carry all of it.
const MaxProjectDecisions = 200

// ListDecisions returns a project's decisions, newest first.
func (d *DB) ListDecisions(project string) ([]ProjectDecision, error) {
	rows, err := d.conn.Query(`SELECT key,value,source,created_at FROM project_decisions WHERE project_id=? ORDER BY created_at DESC,key`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectDecision{}
	for rows.Next() {
		v := ProjectDecision{ProjectID: project}
		var created string
		if err = rows.Scan(&v.Key, &v.Value, &v.Source, &created); err != nil {
			return nil, err
		}
		v.CreatedAt, _ = time.Parse(decisionTime, created) // written by SetDecision in this format
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetDecision returns one decision, or sql.ErrNoRows.
func (d *DB) GetDecision(project, key string) (*ProjectDecision, error) {
	v := &ProjectDecision{ProjectID: project, Key: key}
	var created string
	err := d.conn.QueryRow(`SELECT value,source,created_at FROM project_decisions WHERE project_id=? AND key=?`, project, key).Scan(&v.Value, &v.Source, &created)
	if err != nil {
		return nil, err
	}
	v.CreatedAt, _ = time.Parse(decisionTime, created) // written by SetDecision in this format
	return v, nil
}

// SetDecision saves key's value for the project, replacing an earlier one. When
// keepUser is set, a decision the user made is never replaced (the assistant may not
// overrule the user); it then reports false. A new key in a full log is refused.
func (d *DB) SetDecision(project, key, value, source string, keepUser bool) (bool, error) {
	if source != DecisionByUser && source != DecisionByAssistant {
		return false, fmt.Errorf("decision source must be user or assistant")
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var existing string
	switch err = tx.QueryRow(`SELECT source FROM project_decisions WHERE project_id=? AND key=?`, project, key).Scan(&existing); {
	case errors.Is(err, sql.ErrNoRows):
		var n int
		if err = tx.QueryRow(`SELECT count(*) FROM project_decisions WHERE project_id=?`, project).Scan(&n); err != nil {
			return false, err
		}
		if n >= MaxProjectDecisions {
			return false, ErrDecisionLogFull
		}
	case err != nil:
		return false, err
	case keepUser && existing == DecisionByUser:
		return false, nil
	}
	now := time.Now().UTC().Format(decisionTime)
	if _, err = tx.Exec(`INSERT INTO project_decisions(project_id,key,value,source,created_at) VALUES(?,?,?,?,?)
 ON CONFLICT(project_id,key) DO UPDATE SET value=excluded.value,source=excluded.source,created_at=excluded.created_at`, project, key, value, source, now); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// DeleteDecision removes a decision; it reports whether there was one.
func (d *DB) DeleteDecision(project, key string) (bool, error) {
	res, err := d.conn.Exec(`DELETE FROM project_decisions WHERE project_id=? AND key=?`, project, key)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
