package db

import (
	"encoding/json"
	"fmt"

	"videoflow-go/internal/models"
)

// Failed video attempts stay in render_jobs as records, but they are put away from the
// canvas and the Videos list in two ways: a newer attempt at the same clip replaces
// them by itself, and the owner or the assistant can clear whatever failed ones are left.

func (d *DB) ensureRenderCleanup() error {
	_, err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS cleared_renders(job_id TEXT PRIMARY KEY)`)
	return err
}

func failedRender(status string) bool { return status == models.StatusFailed || status == "canceled" }

// ShownRenderJobs lists a project's render jobs, newest first, without the failed
// attempts that were cleared or that a newer attempt at the same clip replaced.
func (d *DB) ShownRenderJobs(project string) ([]models.RenderJob, error) {
	jobs, err := d.ListRenderJobs(project)
	if err != nil {
		return nil, err
	}
	rows, err := d.conn.Query(`SELECT job_id FROM cleared_renders`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cleared := map[string]bool{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		cleared[id] = true
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	shown, newer := []models.RenderJob{}, map[string]bool{}
	for _, job := range jobs {
		clip := renderClip(job)
		if failedRender(job.Status) && (cleared[job.ID] || newer[clip]) {
			continue
		}
		if clip != "" {
			newer[clip] = true
		}
		shown = append(shown, job)
	}
	return shown, nil
}

// renderClip names the clip a job tried to make: its scene or shot and which part of
// it. Empty for standalone clips, which never replace each other.
func renderClip(job models.RenderJob) string {
	scene, shot := "", ""
	if job.SceneID != nil {
		scene = *job.SceneID
	}
	if job.ShotID != nil {
		shot = *job.ShotID
	}
	if scene == "" && shot == "" {
		return ""
	}
	var request struct {
		Unit struct {
			Index int `json:"index"`
		} `json:"render_unit"`
	}
	_ = json.Unmarshal(job.RequestJSON, &request) // a request without a render unit is unit 0
	return fmt.Sprintf("%s|%s|%d", scene, shot, request.Unit.Index)
}

// FailedRendersShown counts the failed attempts still on show in a project.
func (d *DB) FailedRendersShown(project string) (int, error) {
	jobs, err := d.ShownRenderJobs(project)
	n := 0
	for _, job := range jobs {
		if failedRender(job.Status) {
			n++
		}
	}
	return n, err
}

// ClearFailedRenders puts away every failed attempt of a project and says how many
// were on show. Nothing is deleted, and scenes can still be made again.
func (d *DB) ClearFailedRenders(project string) (int, error) {
	n, err := d.FailedRendersShown(project)
	if err != nil || n == 0 {
		return 0, err
	}
	_, err = d.conn.Exec(`INSERT OR IGNORE INTO cleared_renders(job_id) SELECT id FROM render_jobs WHERE project_id=? AND status IN ('failed','canceled')`, project)
	return n, err
}
