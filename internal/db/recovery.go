package db

import (
	"database/sql"
	"fmt"
	"videoflow-go/internal/models"
)

// ClaimRenderSubmission fences concurrent workers before the one permitted POST.
func (d *DB) ClaimRenderSubmission(id, model string, request []byte) (bool, error) {
	result, err := d.conn.Exec(`UPDATE render_jobs SET status='running',stage='submitting',model=?,request_json=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status IN ('pending','running') AND coalesce(provider_job_id,'')='' AND coalesce(stage,'')<>'submitting'`, model, string(request), id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
func (d *DB) InterruptOperations() error {
	_, err := d.conn.Exec(`UPDATE ops SET status='reconciliation_required',error='Server restarted during this operation. Review saved work before starting another request.',updated_at=CURRENT_TIMESTAMP WHERE status='running'`)
	return err
}

// ResumeUncertainRender goes back to waiting on the service's own job for a render
// whose outcome was unknown, once the service has answered for it. Nothing is sent again.
func (d *DB) ResumeUncertainRender(id string) (bool, error) {
	result, err := d.conn.Exec(`UPDATE render_jobs SET status='running',stage='polling',error=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='reconciliation_required' AND coalesce(provider_job_id,'')<>''`, id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

// FailUncertainRender closes a render whose outcome was unknown, so its scene can be made again.
func (d *DB) FailUncertainRender(id, reason string) (bool, error) {
	result, err := d.conn.Exec(`UPDATE render_jobs SET status='failed',stage='failed',progress=?,error=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='reconciliation_required'`, reason, reason, id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

// CreateRenderRetry serializes retries of the same original, including headerless clients.
func (d *DB) CreateRenderRetry(original string, fresh *models.RenderJob) (bool, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS render_retries(original_job_id TEXT PRIMARY KEY,child_job_id TEXT NOT NULL)`); err != nil {
		return false, err
	}
	var child, status string
	err = tx.QueryRow(`SELECT j.id,j.status FROM render_retries r JOIN render_jobs j ON j.id=r.child_job_id WHERE original_job_id=?`, original).Scan(&child, &status)
	if err == nil && (status == "pending" || status == "running" || status == "reconciliation_required") {
		fresh.ID = child
		fresh.Status = status
		return false, tx.Commit()
	}
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	if err = tx.QueryRow(`SELECT status FROM render_jobs WHERE id=?`, original).Scan(&status); err != nil {
		return false, err
	}
	if status != "failed" && status != "succeeded" {
		return false, fmt.Errorf("original job is still active")
	}
	if err = insertRenderJob(tx, fresh); err != nil {
		return false, err
	}
	if _, err = tx.Exec(`INSERT INTO render_retries(original_job_id,child_job_id)VALUES(?,?) ON CONFLICT(original_job_id) DO UPDATE SET child_job_id=excluded.child_job_id`, original, fresh.ID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// PinRenderEndpoint changes no job state and never reopens a claimed submission.
func (d *DB) PinRenderEndpoint(id, endpoint string) (string, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE render_jobs SET provider_endpoint=? WHERE id=? AND provider_endpoint=''`, endpoint, id); err != nil {
		return "", err
	}
	var pinned string
	if err = tx.QueryRow(`SELECT provider_endpoint FROM render_jobs WHERE id=?`, id).Scan(&pinned); err != nil {
		return "", err
	}
	return pinned, tx.Commit()
}
