package db

import (
	"encoding/json"
	"strings"
	"time"

	"videoflow-go/internal/costs"
)

// CostEntry is one thing a paid service made. Entries are kept even when the scene,
// picture or clip they paid for is deleted later, because the money was still spent.
type CostEntry struct {
	Ref       string    `json:"ref"`
	ProjectID string    `json:"project_id"`
	ScriptID  string    `json:"script_id"`
	SceneID   string    `json:"scene_id"`
	Category  string    `json:"category"` // video, image, voice, text
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	Label     string    `json:"label"`
	Quantity  float64   `json:"quantity"`
	Unit      string    `json:"unit"`
	Amount    float64   `json:"amount"`
	Basis     string    `json:"basis"`
	CreatedAt time.Time `json:"created_at"`
}

func (d *DB) ensureCosts() error {
	_, err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS cost_entries(ref TEXT PRIMARY KEY,project_id TEXT NOT NULL,script_id TEXT NOT NULL DEFAULT '',scene_id TEXT NOT NULL DEFAULT '',category TEXT NOT NULL,provider TEXT NOT NULL,model TEXT NOT NULL DEFAULT '',label TEXT NOT NULL DEFAULT '',quantity REAL NOT NULL DEFAULT 0,unit TEXT NOT NULL DEFAULT '',amount REAL NOT NULL DEFAULT 0,basis TEXT NOT NULL,created_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS cost_entries_project ON cost_entries(project_id,created_at);`)
	return err
}

// RecordCost saves an entry once; a second call with the same ref changes nothing.
func (d *DB) RecordCost(e CostEntry) error {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	_, err := d.conn.Exec(`INSERT OR IGNORE INTO cost_entries(ref,project_id,script_id,scene_id,category,provider,model,label,quantity,unit,amount,basis,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.Ref, e.ProjectID, e.ScriptID, e.SceneID, e.Category, e.Provider, e.Model, e.Label, e.Quantity, e.Unit, e.Amount, e.Basis, e.CreatedAt.UTC().Format(time.RFC3339))
	return err
}

// ListCosts returns one project's entries, or every project's when project is "".
func (d *DB) ListCosts(project string) ([]CostEntry, error) {
	rows, err := d.conn.Query(`SELECT ref,project_id,script_id,scene_id,category,provider,model,label,quantity,unit,amount,basis,created_at FROM cost_entries WHERE project_id=? OR ?='' ORDER BY created_at DESC, ref`, project, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CostEntry{}
	for rows.Next() {
		var e CostEntry
		var created string
		if err = rows.Scan(&e.Ref, &e.ProjectID, &e.ScriptID, &e.SceneID, &e.Category, &e.Provider, &e.Model, &e.Label, &e.Quantity, &e.Unit, &e.Amount, &e.Basis, &created); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, e)
	}
	return out, rows.Err()
}

// CostTotals sums the priced entries of every project, including deleted ones' leftovers.
func (d *DB) CostTotals() (map[string]float64, error) {
	rows, err := d.conn.Query(`SELECT project_id,SUM(amount) FROM cost_entries WHERE basis IN ('metered','rate','estimate') GROUP BY project_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var project string
		var amount float64
		if err = rows.Scan(&project, &amount); err != nil {
			return nil, err
		}
		out[project] = amount
	}
	return out, rows.Err()
}

// AssistantReplies counts finished assistant turns per AI for a project.
func (d *DB) AssistantReplies(project string) (map[string]int, error) {
	rows, err := d.conn.Query(`SELECT s.engine,COUNT(*) FROM director_turns t JOIN director_sessions s ON s.id=t.session_id WHERE s.project_id=? AND t.status='succeeded' GROUP BY s.engine`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var engine string
		var n int
		if err = rows.Scan(&engine, &n); err != nil {
			return nil, err
		}
		out[engine] = n
	}
	return out, rows.Err()
}

// syncCostsFor updates the ledger for the project that owns a row about to be deleted.
func (d *DB) syncCostsFor(table, id string) {
	var project *string
	if d.conn.QueryRow("SELECT project_id FROM "+table+" WHERE id=?", id).Scan(&project) == nil && project != nil {
		_ = d.SyncCosts(*project)
	}
}

// SyncCosts records every finished video, generated picture and voice line of a
// project that is not in the ledger yet. It is safe to call as often as needed.
func (d *DB) SyncCosts(project string) error {
	scenes, err := d.ListScenes(project)
	if err != nil {
		return err
	}
	script, title := map[string]string{}, map[string]string{}
	for _, s := range scenes {
		title[s.ID] = s.Title
		if s.ScriptID != nil {
			script[s.ID] = *s.ScriptID
		}
	}
	settings, _ := d.AppSettingsSnapshot()
	live := costs.LiveFromSettings(settings)
	jobs, err := d.ListRenderJobs(project)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.Provider == "local" || (j.Status != "succeeded" && j.Status != "reconciliation_required") {
			continue
		}
		var req struct {
			Duration   float64 `json:"duration"`
			Resolution string  `json:"resolution"`
			Aspect     string  `json:"aspect_ratio"`
		}
		_ = json.Unmarshal(j.RequestJSON, &req)
		e := CostEntry{Ref: "job:" + j.ID, ProjectID: project, Category: "video", Provider: j.Provider, Model: j.Model, Quantity: req.Duration, Unit: "seconds", CreatedAt: j.CreatedAt, Label: "Video clip"}
		if j.SceneID != nil {
			e.SceneID, e.ScriptID = *j.SceneID, script[*j.SceneID]
			if t := title[*j.SceneID]; t != "" {
				e.Label = t
			}
		}
		if req.Resolution == "" {
			req.Resolution = "720p"
		}
		e.Label += " · " + req.Resolution
		e.Amount, e.Basis = costs.Video(j.Provider, j.Model, req.Resolution, req.Aspect, req.Duration, live)
		if j.Status != "succeeded" {
			// The service accepted the job but the result never arrived, so the charge is unknown.
			e.Ref, e.Basis = "job-unconfirmed:"+j.ID, "unconfirmed"
		}
		if err = d.RecordCost(e); err != nil {
			return err
		}
	}
	assets, err := d.ListAssets(project)
	if err != nil {
		return err
	}
	for _, a := range assets {
		var m struct {
			Provider string            `json:"provider"`
			Model    string            `json:"model"`
			SceneID  string            `json:"scene_id"`
			Text     string            `json:"text"`
			Usage    *costs.ImageUsage `json:"usage"`
		}
		if json.Unmarshal(a.MetadataJSON, &m) != nil || m.Provider == "" {
			continue
		}
		e := CostEntry{Ref: "asset:" + a.ID, ProjectID: project, SceneID: m.SceneID, ScriptID: script[m.SceneID], Provider: m.Provider, Model: m.Model, Label: a.Name, CreatedAt: a.CreatedAt}
		if a.Type == "voiceover" {
			e.Category, e.Quantity, e.Unit = "voice", float64(len([]rune(m.Text))), "characters"
			e.Amount, e.Basis = costs.Voice(m.Provider, m.Model, m.Text)
		} else if strings.HasPrefix(a.FilePath, "storage/assets/") {
			e.Category, e.Quantity, e.Unit = "image", 1, "picture"
			e.Amount, e.Basis = costs.Image(m.Provider, m.Model, m.Usage)
		} else {
			continue
		}
		if err = d.RecordCost(e); err != nil {
			return err
		}
	}
	return nil
}
