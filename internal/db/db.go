package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"videoflow-go/internal/models"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func Open(databasePath string) (*DB, error) {
	conn, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Optimize SQLite for high concurrency WAL mode
	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA foreign_keys = ON;",
	}
	for _, p := range pragmas {
		if _, err := conn.Exec(p); err != nil {
			return nil, fmt.Errorf("failed pragma %s: %w", p, err)
		}
	}

	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}
	if err := d.ensureActiveProject(); err != nil {
		return nil, fmt.Errorf("ensure active project failed: %w", err)
	}
	return d, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS projects (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		is_active BOOLEAN DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS characters (
		id TEXT PRIMARY KEY,
		project_id TEXT,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		appearance TEXT DEFAULT '',
		personality TEXT DEFAULT '',
		visual_rules_json TEXT DEFAULT '[]',
		voice_rules_json TEXT DEFAULT '[]',
		sample_dialogue TEXT DEFAULT '',
		reference_asset_ids_json TEXT DEFAULT '[]',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS assets (
		id TEXT PRIMARY KEY,
		project_id TEXT,
		type TEXT NOT NULL,
		name TEXT NOT NULL,
		file_path TEXT NOT NULL,
		tags_json TEXT DEFAULT '[]',
		description TEXT DEFAULT '',
		character_id TEXT,
		metadata_json TEXT DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS scenes (
		id TEXT PRIMARY KEY,
		project_id TEXT,
		script_id TEXT,
		scene_order INTEGER DEFAULT 0,
		title TEXT DEFAULT '',
		summary TEXT DEFAULT '',
		duration INTEGER DEFAULT 5,
		aspect_ratio TEXT DEFAULT '16:9',
		character_ids_json TEXT DEFAULT '[]',
		asset_ids_json TEXT DEFAULT '[]',
		scene_json TEXT DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS shots (
		id TEXT PRIMARY KEY,
		scene_id TEXT NOT NULL,
		shot_order INTEGER DEFAULT 0,
		duration INTEGER DEFAULT 5,
		prompt TEXT DEFAULT '',
		camera TEXT,
		movement TEXT,
		asset_ids_json TEXT DEFAULT '[]',
		shot_json TEXT DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(scene_id) REFERENCES scenes(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS render_jobs (
		id TEXT PRIMARY KEY,
		project_id TEXT,
		scene_id TEXT,
		shot_id TEXT,
		provider TEXT NOT NULL,
		model TEXT NOT NULL,
		provider_job_id TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		stage TEXT,
		progress TEXT,
		request_json TEXT DEFAULT '{}',
		response_json TEXT DEFAULT '{}',
		error TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS render_outputs (
		id TEXT PRIMARY KEY,
		render_job_id TEXT NOT NULL,
		video_path TEXT NOT NULL,
		thumbnail_path TEXT,
		captioned_path TEXT,
		captions_json TEXT DEFAULT '[]',
		score REAL,
		selected BOOLEAN DEFAULT 0,
		notes TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(render_job_id) REFERENCES render_jobs(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS style_guides (
		id TEXT PRIMARY KEY,
		project_id TEXT,
		name TEXT DEFAULT 'Default',
		style_prompt TEXT DEFAULT '',
		palette TEXT DEFAULT '',
		lighting TEXT DEFAULT '',
		audience TEXT DEFAULT '',
		tone TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS app_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`
	_, err := d.conn.Exec(schema)
	return err
}

func (d *DB) ensureActiveProject() error {
	var count int
	err := d.conn.QueryRow("SELECT COUNT(*) FROM projects WHERE is_active = 1").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		var total int
		if err := d.conn.QueryRow("SELECT COUNT(*) FROM projects").Scan(&total); err != nil {
			return err
		}
		if total == 0 {
			id := "project_" + uuid.New().String()[:8]
			now := time.Now().UTC()
			_, err = d.conn.Exec(
				"INSERT INTO projects (id, name, description, is_active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
				id, "Default Project", "Default workspace", 1, now, now,
			)
			if err != nil {
				return err
			}
			log.Printf("Created initial active project: %s", id)
		} else {
			_, err = d.conn.Exec("UPDATE projects SET is_active = 1 WHERE rowid = (SELECT rowid FROM projects LIMIT 1)")
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *DB) GetActiveProjectID() (string, error) {
	var id string
	err := d.conn.QueryRow("SELECT id FROM projects WHERE is_active = 1 LIMIT 1").Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (d *DB) ListProjects() ([]models.Project, error) {
	rows, err := d.conn.Query("SELECT id, name, description, is_active, created_at, updated_at FROM projects ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		var p models.Project
		var isAct int
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &isAct, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.IsActive = isAct == 1
		projects = append(projects, p)
	}
	return projects, nil
}

func (d *DB) CreateProject(name, description string) (*models.Project, error) {
	id := "project_" + uuid.New().String()[:8]
	now := time.Now().UTC()
	_, err := d.conn.Exec(
		"INSERT INTO projects (id, name, description, is_active, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)",
		id, name, description, now, now,
	)
	if err != nil {
		return nil, err
	}
	return &models.Project{
		ID:          id,
		Name:        name,
		Description: description,
		IsActive:    false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (d *DB) ActivateProject(id string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("UPDATE projects SET is_active = 0"); err != nil {
		return err
	}
	res, err := tx.Exec("UPDATE projects SET is_active = 1, updated_at = ? WHERE id = ?", time.Now().UTC(), id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil || rows == 0 {
		return fmt.Errorf("project not found: %s", id)
	}
	return tx.Commit()
}

func (d *DB) ListCharacters(projectID string) ([]models.Character, error) {
	rows, err := d.conn.Query(`
		SELECT id, project_id, name, description, appearance, personality,
		       visual_rules_json, voice_rules_json, sample_dialogue, reference_asset_ids_json,
		       created_at, updated_at
		FROM characters
		WHERE project_id = ?
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chars []models.Character
	for rows.Next() {
		var c models.Character
		var vr, vor, ra string
		if err := rows.Scan(
			&c.ID, &c.ProjectID, &c.Name, &c.Description, &c.Appearance, &c.Personality,
			&vr, &vor, &c.SampleDialogue, &ra, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		c.VisualRulesJSON = []byte(vr)
		c.VoiceRulesJSON = []byte(vor)
		c.ReferenceAssetIDsJSON = []byte(ra)
		chars = append(chars, c)
	}
	return chars, nil
}

func (d *DB) ListScenes(projectID string) ([]models.Scene, error) {
	rows, err := d.conn.Query(`
		SELECT id, project_id, script_id, scene_order, title, summary, duration,
		       aspect_ratio, character_ids_json, asset_ids_json, scene_json,
		       created_at, updated_at
		FROM scenes
		WHERE project_id = ?
		ORDER BY scene_order ASC, created_at ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scenes []models.Scene
	for rows.Next() {
		var s models.Scene
		var cids, aids, sjson string
		if err := rows.Scan(
			&s.ID, &s.ProjectID, &s.ScriptID, &s.SceneOrder, &s.Title, &s.Summary, &s.Duration,
			&s.AspectRatio, &cids, &aids, &sjson, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		s.CharacterIDsJSON = []byte(cids)
		s.AssetIDsJSON = []byte(aids)
		s.SceneJSON = []byte(sjson)
		scenes = append(scenes, s)
	}
	return scenes, nil
}

func (d *DB) ListShots(sceneID string) ([]models.Shot, error) {
	rows, err := d.conn.Query(`
		SELECT id, scene_id, shot_order, duration, prompt, camera, movement,
		       asset_ids_json, shot_json, created_at, updated_at
		FROM shots
		WHERE scene_id = ?
		ORDER BY shot_order ASC
	`, sceneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shots []models.Shot
	for rows.Next() {
		var s models.Shot
		var aids, sjson string
		if err := rows.Scan(
			&s.ID, &s.SceneID, &s.ShotOrder, &s.Duration, &s.Prompt, &s.Camera, &s.Movement,
			&aids, &sjson, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		s.AssetIDsJSON = []byte(aids)
		s.ShotJSON = []byte(sjson)
		shots = append(shots, s)
	}
	return shots, nil
}

func (d *DB) ListAssets(projectID string) ([]models.Asset, error) {
	rows, err := d.conn.Query(`
		SELECT id, project_id, type, name, file_path, tags_json, description,
		       character_id, metadata_json, created_at, updated_at
		FROM assets
		WHERE project_id = ?
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assets []models.Asset
	for rows.Next() {
		var a models.Asset
		var tags, meta string
		if err := rows.Scan(
			&a.ID, &a.ProjectID, &a.Type, &a.Name, &a.FilePath, &tags, &a.Description,
			&a.CharacterID, &meta, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		a.TagsJSON = []byte(tags)
		a.MetadataJSON = []byte(meta)
		assets = append(assets, a)
	}
	return assets, nil
}

func (d *DB) CreateAsset(a *models.Asset) error {
	if a.ID == "" {
		a.ID = "asset_" + uuid.New().String()[:8]
	}
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now

	tags := string(a.TagsJSON)
	if tags == "" {
		tags = "[]"
	}
	meta := string(a.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}

	_, err := d.conn.Exec(`
		INSERT INTO assets (id, project_id, type, name, file_path, tags_json, description, character_id, metadata_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.ID, a.ProjectID, a.Type, a.Name, a.FilePath, tags, a.Description, a.CharacterID, meta, now, now)
	return err
}

func (d *DB) GetRenderJob(id string) (*models.RenderJob, error) {
	row := d.conn.QueryRow(`
		SELECT id, project_id, scene_id, shot_id, provider, model, provider_job_id,
		       status, stage, progress, request_json, response_json, error, created_at, updated_at
		FROM render_jobs
		WHERE id = ?
	`, id)
	var j models.RenderJob
	var req, resp string
	if err := row.Scan(
		&j.ID, &j.ProjectID, &j.SceneID, &j.ShotID, &j.Provider, &j.Model, &j.ProviderJobID,
		&j.Status, &j.Stage, &j.Progress, &req, &resp, &j.Error, &j.CreatedAt, &j.UpdatedAt,
	); err != nil {
		return nil, err
	}
	j.RequestJSON = []byte(req)
	j.ResponseJSON = []byte(resp)
	return &j, nil
}

func (d *DB) CreateRenderJob(j *models.RenderJob) error {
	if j.ID == "" {
		j.ID = "job_" + uuid.New().String()[:8]
	}
	now := time.Now().UTC()
	j.CreatedAt = now
	j.UpdatedAt = now

	req := string(j.RequestJSON)
	if req == "" {
		req = "{}"
	}
	resp := string(j.ResponseJSON)
	if resp == "" {
		resp = "{}"
	}

	_, err := d.conn.Exec(`
		INSERT INTO render_jobs (id, project_id, scene_id, shot_id, provider, model, provider_job_id, status, stage, progress, request_json, response_json, error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, j.ID, j.ProjectID, j.SceneID, j.ShotID, j.Provider, j.Model, j.ProviderJobID, j.Status, j.Stage, j.Progress, req, resp, j.Error, now, now)
	return err
}

func (d *DB) UpdateRenderJob(j *models.RenderJob) error {
	now := time.Now().UTC()
	j.UpdatedAt = now

	_, err := d.conn.Exec(`
		UPDATE render_jobs
		SET status = ?, stage = ?, progress = ?, provider_job_id = ?, response_json = ?, error = ?, updated_at = ?
		WHERE id = ?
	`, j.Status, j.Stage, j.Progress, j.ProviderJobID, string(j.ResponseJSON), j.Error, now, j.ID)
	return err
}

func (d *DB) ListPendingRenderJobs() ([]models.RenderJob, error) {
	rows, err := d.conn.Query(`
		SELECT id, project_id, scene_id, shot_id, provider, model, provider_job_id,
		       status, stage, progress, request_json, response_json, error, created_at, updated_at
		FROM render_jobs
		WHERE status IN ('pending', 'running')
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.RenderJob
	for rows.Next() {
		var j models.RenderJob
		var req, resp string
		if err := rows.Scan(
			&j.ID, &j.ProjectID, &j.SceneID, &j.ShotID, &j.Provider, &j.Model, &j.ProviderJobID,
			&j.Status, &j.Stage, &j.Progress, &req, &resp, &j.Error, &j.CreatedAt, &j.UpdatedAt,
		); err != nil {
			return nil, err
		}
		j.RequestJSON = []byte(req)
		j.ResponseJSON = []byte(resp)
		jobs = append(jobs, j)
	}
	return jobs, nil
}
