package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"strings"
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

	// SQLite permits one writer; keep all operations on the connection whose
	// busy timeout and foreign-key pragmas are initialized below. New pooled
	// connections otherwise inherit neither setting and fail concurrent writes.
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

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
	if err := d.ensureDirectorWorkflow(); err != nil {
		return nil, err
	}
	if err := d.EnsureVoiceWorkflow(); err != nil {
		return nil, err
	}
	if err := d.ensureMediaProviders(); err != nil {
		return nil, err
	}
	if err := d.ensureRenderCleanup(); err != nil {
		return nil, err
	}
	if err := d.ensureCosts(); err != nil {
		return nil, err
	}
	if err := d.ensureRequestReplay(); err != nil {
		return nil, err
	}
	if err := d.runMigrations(); err != nil {
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

// migrate creates the original tables that are missing; see baseSchema.
func (d *DB) migrate() error {
	for _, tables := range baseSchema {
		if _, err := d.conn.Exec(tables); err != nil {
			return err
		}
	}
	return nil
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

// --- Projects ---

func (d *DB) GetActiveProjectID() (string, error) {
	var id string
	err := d.conn.QueryRow("SELECT id FROM projects WHERE is_active = 1 LIMIT 1").Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (d *DB) GetActiveProject() (*models.Project, error) {
	var p models.Project
	var isAct int
	err := d.conn.QueryRow("SELECT id, name, description, is_active, created_at, updated_at FROM projects WHERE is_active = 1 LIMIT 1").
		Scan(&p.ID, &p.Name, &p.Description, &isAct, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.IsActive = isAct == 1
	return &p, nil
}

func (d *DB) ListProjects() ([]models.Project, error) {
	rows, err := d.conn.Query(`
 SELECT p.id, p.name, p.description, p.is_active, p.created_at, p.updated_at,
 (SELECT COUNT(*) FROM scenes WHERE project_id = p.id),
 (SELECT COUNT(*) FROM characters WHERE project_id = p.id),
 (SELECT COUNT(*) FROM assets WHERE project_id = p.id),
 COALESCE((SELECT file_path FROM assets WHERE project_id = p.id
   AND (lower(file_path) LIKE '%.png' OR lower(file_path) LIKE '%.jpg' OR lower(file_path) LIKE '%.jpeg' OR lower(file_path) LIKE '%.webp')
   ORDER BY CASE type WHEN 'storyboard' THEN 0 WHEN 'location' THEN 1 ELSE 2 END, created_at DESC LIMIT 1), '')
 FROM projects p ORDER BY p.created_at DESC
 `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		var p models.Project
		var isAct int
		var counts models.ProjectCounts
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &isAct, &p.CreatedAt, &p.UpdatedAt, &counts.Scenes, &counts.Characters, &counts.Assets, &p.Cover); err != nil {
			return nil, err
		}
		p.Counts = &counts
		p.IsActive = isAct == 1
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (d *DB) GetProject(id string) (*models.Project, error) {
	var p models.Project
	var isAct int
	err := d.conn.QueryRow("SELECT id, name, description, is_active, created_at, updated_at FROM projects WHERE id = ?", id).
		Scan(&p.ID, &p.Name, &p.Description, &isAct, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.IsActive = isAct == 1
	return &p, nil
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

func (d *DB) UpdateProject(id, name, description string) error {
	now := time.Now().UTC()
	_, err := d.conn.Exec("UPDATE projects SET name = ?, description = ?, updated_at = ? WHERE id = ?", name, description, now, id)
	return err
}

var ErrProjectBusy = errors.New("project has work in progress; wait for it to finish before deleting")

func (d *DB) DeleteProject(id string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active, busy bool
	err = tx.QueryRow(`SELECT is_active,
 EXISTS(SELECT 1 FROM ops WHERE project_id=? AND status='running') OR
 EXISTS(SELECT 1 FROM render_jobs WHERE project_id=? AND status IN ('pending','queued','submitted','running','reconciliation_required')) OR
 EXISTS(SELECT 1 FROM voice_tasks WHERE project_id=? AND status IN ('submitting','queued','running','uncertain')) OR
 EXISTS(SELECT 1 FROM director_sessions WHERE project_id=? AND status='running')
 FROM projects WHERE id=?`, id, id, id, id, id).Scan(&active, &busy)
	if err != nil {
		return err
	}
	if busy {
		return ErrProjectBusy
	}
	for _, query := range []string{"DELETE FROM ops WHERE project_id=?", "DELETE FROM projects WHERE id=?"} {
		if _, err = tx.Exec(query, id); err != nil {
			return err
		}
	}
	if active {
		res, err := tx.Exec(`UPDATE projects SET is_active=1 WHERE id=(SELECT id FROM projects ORDER BY updated_at DESC,id LIMIT 1)`)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			now := time.Now().UTC()
			if _, err = tx.Exec(`INSERT INTO projects(id,name,description,is_active,created_at,updated_at) VALUES(?, 'Default Project','Default workspace',1,?,?)`, "project_"+uuid.New().String()[:8], now, now); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
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

// --- Characters ---

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

func (d *DB) GetCharacter(id string) (*models.Character, error) {
	var c models.Character
	var vr, vor, ra string
	err := d.conn.QueryRow(`
		SELECT id, project_id, name, description, appearance, personality,
		       visual_rules_json, voice_rules_json, sample_dialogue, reference_asset_ids_json,
		       created_at, updated_at
		FROM characters WHERE id = ?
	`, id).Scan(
		&c.ID, &c.ProjectID, &c.Name, &c.Description, &c.Appearance, &c.Personality,
		&vr, &vor, &c.SampleDialogue, &ra, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	c.VisualRulesJSON = []byte(vr)
	c.VoiceRulesJSON = []byte(vor)
	c.ReferenceAssetIDsJSON = []byte(ra)
	return &c, nil
}

func (d *DB) CreateCharacter(c *models.Character) error { return insertCharacter(d.conn, c) }

func insertCharacter(conn sqlExecutor, c *models.Character) error {
	if c.ID == "" {
		c.ID = "char_" + uuid.New().String()[:8]
	}
	now := time.Now().UTC()
	c.CreatedAt = now
	c.UpdatedAt = now

	vr := string(c.VisualRulesJSON)
	if vr == "" {
		vr = "[]"
	}
	vor := string(c.VoiceRulesJSON)
	if vor == "" {
		vor = "[]"
	}
	ra := string(c.ReferenceAssetIDsJSON)
	if ra == "" {
		ra = "[]"
	}

	_, err := conn.Exec(`
		INSERT INTO characters (id, project_id, name, description, appearance, personality, visual_rules_json, voice_rules_json, sample_dialogue, reference_asset_ids_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID, c.ProjectID, c.Name, c.Description, c.Appearance, c.Personality, vr, vor, c.SampleDialogue, ra, now, now)
	return err
}

func (d *DB) UpdateCharacter(c *models.Character) error {
	expected := c.UpdatedAt
	now := time.Now().UTC()
	c.UpdatedAt = now
	vr := string(c.VisualRulesJSON)
	if vr == "" {
		vr = "[]"
	}
	vor := string(c.VoiceRulesJSON)
	if vor == "" {
		vor = "[]"
	}
	ra := string(c.ReferenceAssetIDsJSON)
	if ra == "" {
		ra = "[]"
	}

	result, err := d.conn.Exec(`
		UPDATE characters
		SET name = ?, description = ?, appearance = ?, personality = ?, visual_rules_json = ?, voice_rules_json = ?, sample_dialogue = ?, reference_asset_ids_json = ?, updated_at = ?
		WHERE id = ? AND updated_at = ?
	`, c.Name, c.Description, c.Appearance, c.Personality, vr, vor, c.SampleDialogue, ra, now, c.ID, expected)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return fmt.Errorf("record changed during this operation; reload before retrying")
	}
	return nil
}

func (d *DB) DeleteCharacter(id string) error {
	_, err := d.conn.Exec("DELETE FROM characters WHERE id = ?", id)
	return err
}

// --- Scenes & Scripts ---

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

func (d *DB) GetScene(id string) (*models.Scene, error) {
	var s models.Scene
	var cids, aids, sjson string
	err := d.conn.QueryRow(`
		SELECT id, project_id, script_id, scene_order, title, summary, duration,
		       aspect_ratio, character_ids_json, asset_ids_json, scene_json,
		       created_at, updated_at
		FROM scenes WHERE id = ?
	`, id).Scan(
		&s.ID, &s.ProjectID, &s.ScriptID, &s.SceneOrder, &s.Title, &s.Summary, &s.Duration,
		&s.AspectRatio, &cids, &aids, &sjson, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	s.CharacterIDsJSON = []byte(cids)
	s.AssetIDsJSON = []byte(aids)
	s.SceneJSON = []byte(sjson)
	return &s, nil
}

func (d *DB) CreateScene(s *models.Scene) error { return insertScene(d.conn, s) }
func insertScene(conn sqlExecutor, s *models.Scene) error {
	if s.ID == "" {
		s.ID = "scene_" + uuid.New().String()[:8]
	}
	now := time.Now().UTC()
	s.CreatedAt = now
	s.UpdatedAt = now

	cids := string(s.CharacterIDsJSON)
	if cids == "" {
		cids = "[]"
	}
	aids := string(s.AssetIDsJSON)
	if aids == "" {
		aids = "[]"
	}
	sjson := string(s.SceneJSON)
	if sjson == "" {
		sjson = "{}"
	}

	_, err := conn.Exec(`
		INSERT INTO scenes (id, project_id, script_id, scene_order, title, summary, duration, aspect_ratio, character_ids_json, asset_ids_json, scene_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.ID, s.ProjectID, s.ScriptID, s.SceneOrder, s.Title, s.Summary, s.Duration, s.AspectRatio, cids, aids, sjson, now, now)
	return err
}

func (d *DB) UpdateScene(s *models.Scene) error {
	expected := s.UpdatedAt
	now := time.Now().UTC()
	s.UpdatedAt = now
	cids := string(s.CharacterIDsJSON)
	if cids == "" {
		cids = "[]"
	}
	aids := string(s.AssetIDsJSON)
	if aids == "" {
		aids = "[]"
	}
	sjson := string(s.SceneJSON)
	if sjson == "" {
		sjson = "{}"
	}

	result, err := d.conn.Exec(`
		UPDATE scenes
		SET title = ?, summary = ?, duration = ?, aspect_ratio = ?, scene_order = ?, character_ids_json = ?, asset_ids_json = ?, scene_json = ?, updated_at = ?
		WHERE id = ? AND updated_at = ?
	`, s.Title, s.Summary, s.Duration, s.AspectRatio, s.SceneOrder, cids, aids, sjson, now, s.ID, expected)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return fmt.Errorf("record changed during this operation; reload before retrying")
	}
	return nil
}

func (d *DB) DeleteScene(id string) error {
	d.syncCostsFor("scenes", id)
	_, err := d.conn.Exec("DELETE FROM scenes WHERE id = ?", id)
	return err
}

// --- Shots ---

func (d *DB) ListShots(sceneID string) ([]models.Shot, error) {
	rows, err := d.conn.Query(`
		SELECT id, scene_id, shot_order, duration, prompt, camera, movement,
		       asset_ids_json, shot_json, created_at, updated_at
		FROM shots
		WHERE scene_id = ?
		ORDER BY shot_order ASC, created_at ASC
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

func (d *DB) GetShot(id string) (*models.Shot, error) {
	var s models.Shot
	var aids, sjson string
	err := d.conn.QueryRow(`
		SELECT id, scene_id, shot_order, duration, prompt, camera, movement,
		       asset_ids_json, shot_json, created_at, updated_at
		FROM shots WHERE id = ?
	`, id).Scan(
		&s.ID, &s.SceneID, &s.ShotOrder, &s.Duration, &s.Prompt, &s.Camera, &s.Movement,
		&aids, &sjson, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	s.AssetIDsJSON = []byte(aids)
	s.ShotJSON = []byte(sjson)
	return &s, nil
}

func (d *DB) CreateShot(s *models.Shot) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = insertShot(tx, s); err != nil {
		return err
	}
	if _, err = tx.Exec("UPDATE scenes SET updated_at=? WHERE id=?", time.Now().UTC(), s.SceneID); err != nil {
		return err
	}
	return tx.Commit()
}
func insertShot(conn sqlExecutor, s *models.Shot) error {
	if s.ID == "" {
		s.ID = "shot_" + uuid.New().String()[:8]
	}
	now := time.Now().UTC()
	s.CreatedAt = now
	s.UpdatedAt = now

	aids := string(s.AssetIDsJSON)
	if aids == "" {
		aids = "[]"
	}
	sjson := string(s.ShotJSON)
	if sjson == "" {
		sjson = "{}"
	}

	_, err := conn.Exec(`
		INSERT INTO shots (id, scene_id, shot_order, duration, prompt, camera, movement, asset_ids_json, shot_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.ID, s.SceneID, s.ShotOrder, s.Duration, s.Prompt, s.Camera, s.Movement, aids, sjson, now, now)
	return err
}

func (d *DB) UpdateShot(s *models.Shot) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	aids := string(s.AssetIDsJSON)
	if aids == "" {
		aids = "[]"
	}
	data := string(s.ShotJSON)
	if data == "" {
		data = "{}"
	}
	result, err := tx.Exec(`UPDATE shots SET prompt=?,duration=?,camera=?,movement=?,shot_order=?,asset_ids_json=?,shot_json=?,updated_at=? WHERE id=? AND updated_at=?`, s.Prompt, s.Duration, s.Camera, s.Movement, s.ShotOrder, aids, data, now, s.ID, s.UpdatedAt)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return fmt.Errorf("shot changed; reload before saving")
	}
	if _, err = tx.Exec(`UPDATE scenes SET updated_at=? WHERE id=(SELECT scene_id FROM shots WHERE id=?)`, now, s.ID); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.UpdatedAt = now
	return nil
}
func (d *DB) DeleteShot(id string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE scenes SET updated_at=? WHERE id=(SELECT scene_id FROM shots WHERE id=?)`, time.Now().UTC(), id); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM shots WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// --- Assets ---

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

func (d *DB) GetAsset(id string) (*models.Asset, error) {
	var a models.Asset
	var tags, meta string
	err := d.conn.QueryRow(`
		SELECT id, project_id, type, name, file_path, tags_json, description,
		       character_id, metadata_json, created_at, updated_at
		FROM assets WHERE id = ?
	`, id).Scan(
		&a.ID, &a.ProjectID, &a.Type, &a.Name, &a.FilePath, &tags, &a.Description,
		&a.CharacterID, &meta, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	a.TagsJSON = []byte(tags)
	a.MetadataJSON = []byte(meta)
	return &a, nil
}

type sqlExecutor interface {
	Exec(string, ...any) (sql.Result, error)
}

func (d *DB) CreateAsset(a *models.Asset) error { return insertAsset(d.conn, a) }

// CreateUploadedAsset commits the asset and its character reference together.
func (d *DB) CreateUploadedAsset(a *models.Asset) error {
	if a.CharacterID == nil {
		return d.CreateAsset(a)
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var projectID string
	if err = tx.QueryRow(`SELECT project_id FROM characters WHERE id=?`, *a.CharacterID).Scan(&projectID); err != nil {
		return err
	}
	if a.ProjectID == nil || projectID != *a.ProjectID {
		return fmt.Errorf("character is not in the active project")
	}
	if err = insertAsset(tx, a); err != nil {
		return err
	}
	extension := strings.ToLower(filepath.Ext(a.FilePath))
	if extension == ".png" || extension == ".jpg" || extension == ".jpeg" || extension == ".webp" || extension == ".gif" || extension == ".svg" {
		if _, err = tx.Exec(`UPDATE characters SET reference_asset_ids_json=json_insert(reference_asset_ids_json,'$[#]',?),updated_at=? WHERE id=?`, a.ID, time.Now().UTC(), *a.CharacterID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertAsset(conn sqlExecutor, a *models.Asset) error {
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

	_, err := conn.Exec(`
		INSERT INTO assets (id, project_id, type, name, file_path, tags_json, description, character_id, metadata_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.ID, a.ProjectID, a.Type, a.Name, a.FilePath, tags, a.Description, a.CharacterID, meta, now, now)
	return err
}

func (d *DB) UpdateAsset(a *models.Asset) error {
	now := time.Now().UTC()
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
		UPDATE assets
		SET type = ?, name = ?, tags_json = ?, description = ?, character_id = ?, metadata_json = ?, updated_at = ?
		WHERE id = ?
	`, a.Type, a.Name, tags, a.Description, a.CharacterID, meta, now, a.ID)
	return err
}

func (d *DB) DeleteAsset(id string) error {
	d.syncCostsFor("assets", id)
	_, err := d.conn.Exec("DELETE FROM assets WHERE id = ?", id)
	return err
}

// --- Scripts ---

func (d *DB) ListScripts(projectID string) ([]models.Script, error) {
	rows, err := d.conn.Query("SELECT id, project_id, idea, title, summary, draft_json, created_at, updated_at FROM scripts WHERE project_id = ? ORDER BY created_at DESC", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scripts []models.Script
	for rows.Next() {
		var s models.Script
		var draft string
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.Idea, &s.Title, &s.Summary, &draft, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.DraftJSON = []byte(draft)
		scripts = append(scripts, s)
	}
	return scripts, nil
}

func (d *DB) CreateScript(s *models.Script) error { return insertScript(d.conn, s) }

// SetScriptSoundPlan keeps a story's sound plan in its draft under "sound_plan". The
// story's other fields and its updated time are left as they are.
func (d *DB) SetScriptSoundPlan(id string, plan []byte) error {
	result, err := d.conn.Exec(`UPDATE scripts SET draft_json=json_set(CASE WHEN json_valid(draft_json) THEN draft_json ELSE '{}' END,'$.sound_plan',json(?)) WHERE id=?`, string(plan), id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func insertScript(conn sqlExecutor, s *models.Script) error {
	if s.ID == "" {
		s.ID = "script_" + uuid.New().String()[:8]
	}
	now := time.Now().UTC()
	s.CreatedAt = now
	s.UpdatedAt = now
	draft := string(s.DraftJSON)
	if draft == "" {
		draft = "{}"
	}
	_, err := conn.Exec(
		"INSERT INTO scripts (id, project_id, idea, title, summary, draft_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		s.ID, s.ProjectID, s.Idea, s.Title, s.Summary, draft, now, now,
	)
	return err
}

// --- Style Guides ---

func (d *DB) GetStyleGuide(projectID string) (*models.StyleGuide, error) {
	var s models.StyleGuide
	err := d.conn.QueryRow(`
		SELECT id, project_id, name, style_prompt, palette, lighting, audience, tone, created_at, updated_at
		FROM style_guides WHERE project_id = ? LIMIT 1
	`, projectID).Scan(&s.ID, &s.ProjectID, &s.Name, &s.StylePrompt, &s.Palette, &s.Lighting, &s.Audience, &s.Tone, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (d *DB) UpsertStyleGuide(s *models.StyleGuide) error {
	if s.ProjectID == nil {
		return fmt.Errorf("style project is required")
	}
	if s.ID != "" {
		var owner string
		err := d.conn.QueryRow(`SELECT project_id FROM style_guides WHERE id=?`, s.ID).Scan(&owner)
		if err == nil && owner != *s.ProjectID {
			return fmt.Errorf("style belongs to another project")
		}
	}
	if existing, err := d.GetStyleGuide(*s.ProjectID); err == nil {
		s.ID = existing.ID
		s.CreatedAt = existing.CreatedAt
	}
	return upsertStyleGuide(d.conn, s)
}

func upsertStyleGuide(conn sqlExecutor, s *models.StyleGuide) error {
	now := time.Now().UTC()
	if s.ID == "" {
		s.ID = "style_" + uuid.New().String()[:8]
		s.CreatedAt = now
	}
	s.UpdatedAt = now

	_, err := conn.Exec(`
		INSERT INTO style_guides (id, project_id, name, style_prompt, palette, lighting, audience, tone, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			style_prompt = excluded.style_prompt,
			palette = excluded.palette,
			lighting = excluded.lighting,
			audience = excluded.audience,
			tone = excluded.tone,
			updated_at = excluded.updated_at
	`, s.ID, s.ProjectID, s.Name, s.StylePrompt, s.Palette, s.Lighting, s.Audience, s.Tone, s.CreatedAt, s.UpdatedAt)
	return err
}

// --- Render Jobs & Outputs ---

func (d *DB) GetRenderJob(id string) (*models.RenderJob, error) {
	row := d.conn.QueryRow(`
		SELECT id, project_id, scene_id, shot_id, provider, model, provider_endpoint, provider_job_id,
		       status, stage, progress, request_json, response_json, error, created_at, updated_at
		FROM render_jobs
		WHERE id = ?
	`, id)
	var j models.RenderJob
	var req, resp string
	if err := row.Scan(
		&j.ID, &j.ProjectID, &j.SceneID, &j.ShotID, &j.Provider, &j.Model, &j.ProviderEndpoint, &j.ProviderJobID,
		&j.Status, &j.Stage, &j.Progress, &req, &resp, &j.Error, &j.CreatedAt, &j.UpdatedAt,
	); err != nil {
		return nil, err
	}
	j.RequestJSON = []byte(req)
	j.ResponseJSON = []byte(resp)
	return &j, nil
}

func (d *DB) ListRenderJobs(projectID string) ([]models.RenderJob, error) {
	rows, err := d.conn.Query(`
		SELECT id, project_id, scene_id, shot_id, provider, model, provider_endpoint, provider_job_id,
		       status, stage, progress, request_json, response_json, error, created_at, updated_at
		FROM render_jobs
		WHERE project_id = ?
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.RenderJob
	for rows.Next() {
		var j models.RenderJob
		var req, resp string
		if err := rows.Scan(
			&j.ID, &j.ProjectID, &j.SceneID, &j.ShotID, &j.Provider, &j.Model, &j.ProviderEndpoint, &j.ProviderJobID,
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

func (d *DB) CreateRenderJob(j *models.RenderJob) error { return insertRenderJob(d.conn, j) }
func insertRenderJob(conn sqlExecutor, j *models.RenderJob) error {
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

	_, err := conn.Exec(`
		INSERT INTO render_jobs (id, project_id, scene_id, shot_id, provider, model, provider_endpoint, provider_job_id, status, stage, progress, request_json, response_json, error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, j.ID, j.ProjectID, j.SceneID, j.ShotID, j.Provider, j.Model, j.ProviderEndpoint, j.ProviderJobID, j.Status, j.Stage, j.Progress, req, resp, j.Error, now, now)
	return err
}

func (d *DB) UpdateRenderJob(j *models.RenderJob) error {
	now := time.Now().UTC()
	j.UpdatedAt = now

	_, err := d.conn.Exec(`
		UPDATE render_jobs
		SET status = ?, stage = ?, progress = ?, provider_job_id = ?, response_json = ?, error = ?, model = ?, provider = ?, provider_endpoint = ?, request_json = ?, updated_at = ?
		WHERE id = ?
	`, j.Status, j.Stage, j.Progress, j.ProviderJobID, string(j.ResponseJSON), j.Error, j.Model, j.Provider, j.ProviderEndpoint, string(j.RequestJSON), now, j.ID)
	return err
}

func (d *DB) ListPendingRenderJobs() ([]models.RenderJob, error) {
	rows, err := d.conn.Query(`
		SELECT id, project_id, scene_id, shot_id, provider, model, provider_endpoint, provider_job_id,
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
			&j.ID, &j.ProjectID, &j.SceneID, &j.ShotID, &j.Provider, &j.Model, &j.ProviderEndpoint, &j.ProviderJobID,
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

func (d *DB) ListOutputsForJob(jobID string) ([]models.RenderOutput, error) {
	rows, err := d.conn.Query(`
		SELECT id, render_job_id, video_path, thumbnail_path, captioned_path, captions_json, score, qa_json, selected, notes, created_at, updated_at
		FROM render_outputs
		WHERE render_job_id = ?
		ORDER BY created_at DESC
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var outputs []models.RenderOutput
	for rows.Next() {
		var o models.RenderOutput
		var caps, qa string
		var sel int
		if err := rows.Scan(&o.ID, &o.RenderJobID, &o.VideoPath, &o.ThumbnailPath, &o.CaptionedPath, &caps, &o.Score, &qa, &sel, &o.Notes, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		o.CaptionsJSON = []byte(caps)
		o.QAJSON = []byte(qa)
		o.Selected = sel == 1
		outputs = append(outputs, o)
	}
	return outputs, nil
}

// --- Ops ---

func (d *DB) CreateOp(op *models.Op) error {
	if op.ID == "" {
		op.ID = "op_" + uuid.New().String()[:8]
	}
	now := time.Now().UTC()
	op.CreatedAt = now
	op.UpdatedAt = now

	res := string(op.ResultJSON)
	if res == "" {
		res = "{}"
	}

	_, err := d.conn.Exec(`
		INSERT INTO ops (id, kind, status, scene_id, output_id, project_id, error, result_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, op.ID, op.Kind, op.Status, op.SceneID, op.OutputID, op.ProjectID, op.Error, res, now, now)
	return err
}

func (d *DB) GetOp(id string) (*models.Op, error) {
	var op models.Op
	var res string
	err := d.conn.QueryRow(`
		SELECT id, kind, status, scene_id, output_id, project_id, error, result_json, created_at, updated_at
		FROM ops WHERE id = ?
	`, id).Scan(&op.ID, &op.Kind, &op.Status, &op.SceneID, &op.OutputID, &op.ProjectID, &op.Error, &res, &op.CreatedAt, &op.UpdatedAt)
	if err != nil {
		return nil, err
	}
	op.ResultJSON = []byte(res)
	return &op, nil
}

func (d *DB) UpdateOp(id, status string, resultJSON json.RawMessage, errorMsg *string) error {
	now := time.Now().UTC()
	res := string(resultJSON)
	if res == "" {
		res = "{}"
	}
	_, err := d.conn.Exec("UPDATE ops SET status = ?, result_json = ?, error = ?, updated_at = ? WHERE id = ?", status, res, errorMsg, now, id)
	return err
}

func (d *DB) ListOps(projectID string, limit int) ([]models.Op, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := d.conn.Query(`
		SELECT id, kind, status, scene_id, output_id, project_id, error, result_json, created_at, updated_at
		FROM ops
		WHERE project_id = ? OR project_id IS NULL
		ORDER BY created_at DESC
		LIMIT ?
	`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ops []models.Op
	for rows.Next() {
		var op models.Op
		var res string
		if err := rows.Scan(&op.ID, &op.Kind, &op.Status, &op.SceneID, &op.OutputID, &op.ProjectID, &op.Error, &res, &op.CreatedAt, &op.UpdatedAt); err != nil {
			return nil, err
		}
		op.ResultJSON = []byte(res)
		ops = append(ops, op)
	}
	return ops, nil
}

func (d *DB) ListOpsFiltered(projectID, kind string, limit int) ([]models.Op, error) {
	if limit <= 0 && limit != -1 {
		limit = 20
	}
	query := "SELECT id, kind, status, scene_id, output_id, project_id, error, result_json, created_at, updated_at FROM ops WHERE 1=1"
	args := []interface{}{}
	if projectID != "" {
		query += " AND (project_id = ? OR project_id IS NULL)"
		args = append(args, projectID)
	}
	if kind != "" {
		query += " AND kind = ?"
		args = append(args, kind)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ops := []models.Op{}
	for rows.Next() {
		var op models.Op
		var res string
		if err := rows.Scan(&op.ID, &op.Kind, &op.Status, &op.SceneID, &op.OutputID, &op.ProjectID, &op.Error, &res, &op.CreatedAt, &op.UpdatedAt); err != nil {
			return nil, err
		}
		op.ResultJSON = []byte(res)
		ops = append(ops, op)
	}
	return ops, nil
}

// --- App Settings ---

func (d *DB) GetAppSetting(key string) (string, error) {
	var val string
	err := d.conn.QueryRow("SELECT value FROM app_settings WHERE key = ?", key).Scan(&val)
	if err != nil {
		return "", err
	}
	return val, nil
}

func (d *DB) SetAppSetting(key, val string) error {
	_, err := d.conn.Exec("INSERT INTO app_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, val)
	return err
}

// --- Provider Secrets & Agent Settings ---

func (d *DB) GetProviderSecret(provider string) (apiKey string, baseURL *string, fromDB bool, err error) {
	var key, url sql.NullString
	err = d.conn.QueryRow("SELECT api_key, base_url FROM provider_secrets WHERE provider = ?", provider).Scan(&key, &url)
	if err == sql.ErrNoRows {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	if url.Valid {
		b := url.String
		baseURL = &b
	}
	return key.String, baseURL, true, nil
}

func (d *DB) SetProviderSecret(provider, apiKey string, baseURL *string) error {
	now := time.Now().UTC()
	var urlVal interface{}
	if baseURL != nil && *baseURL != "" {
		urlVal = *baseURL
	}
	_, err := d.conn.Exec(`
		INSERT INTO provider_secrets (provider, api_key, base_url, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(provider) DO UPDATE SET
			api_key = CASE WHEN ? = '' AND excluded.api_key = '' THEN '' WHEN ? != '' THEN excluded.api_key ELSE provider_secrets.api_key END,
			base_url = CASE WHEN ? IS NOT NULL THEN excluded.base_url ELSE provider_secrets.base_url END,
			updated_at = excluded.updated_at
	`, provider, apiKey, urlVal, now, apiKey, apiKey, urlVal)
	return err
}

// --- Connections ---

func (d *DB) CreateConnection(c *models.Connection) error {
	now := time.Now().UTC()
	c.CreatedAt = now
	_, err := d.conn.Exec(`
		INSERT INTO connections (name, label, preset, protocol, base_url, model, mode, vision, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.Name, c.Label, c.Preset, c.Protocol, c.BaseURL, c.Model, c.Mode, c.Vision, c.CreatedAt)
	return err
}

func (d *DB) ListConnections() ([]models.Connection, error) {
	rows, err := d.conn.Query("SELECT name, label, preset, protocol, base_url, model, mode, vision, created_at FROM connections ORDER BY created_at ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Connection
	for rows.Next() {
		var c models.Connection
		var bUrl sql.NullString
		if err := rows.Scan(&c.Name, &c.Label, &c.Preset, &c.Protocol, &bUrl, &c.Model, &c.Mode, &c.Vision, &c.CreatedAt); err != nil {
			return nil, err
		}
		if bUrl.Valid {
			u := bUrl.String
			c.BaseURL = &u
		}
		list = append(list, c)
	}
	return list, nil
}

// --- Outputs ---

func (d *DB) CreateRenderOutput(o *models.RenderOutput) error { return insertRenderOutput(d.conn, o) }
func insertRenderOutput(conn sqlExecutor, o *models.RenderOutput) error {
	if o.ID == "" {
		o.ID = "output_" + uuid.New().String()[:8]
	}
	now := time.Now().UTC()
	o.CreatedAt = now
	o.UpdatedAt = now

	caps := string(o.CaptionsJSON)
	if caps == "" {
		caps = "[]"
	}
	qa := string(o.QAJSON)
	if qa == "" {
		qa = "{}"
	}
	sel := 0
	if o.Selected {
		sel = 1
	}

	_, err := conn.Exec(`
		INSERT INTO render_outputs (
			id, render_job_id, video_path, thumbnail_path, captioned_path,
			captions_json, score, qa_json, selected, notes, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, o.ID, o.RenderJobID, o.VideoPath, o.ThumbnailPath, o.CaptionedPath,
		caps, o.Score, qa, sel, o.Notes, now, now)
	return err
}

func (d *DB) GetRenderOutput(id string) (*models.RenderOutput, error) {
	row := d.conn.QueryRow(`
		SELECT id, render_job_id, video_path, thumbnail_path, captioned_path, captions_json, score, qa_json, selected, notes, created_at, updated_at
		FROM render_outputs
		WHERE id = ?
	`, id)

	var o models.RenderOutput
	var caps, qa string
	var sel int
	if err := row.Scan(&o.ID, &o.RenderJobID, &o.VideoPath, &o.ThumbnailPath, &o.CaptionedPath, &caps, &o.Score, &qa, &sel, &o.Notes, &o.CreatedAt, &o.UpdatedAt); err != nil {
		return nil, err
	}
	o.CaptionsJSON = []byte(caps)
	o.QAJSON = []byte(qa)
	o.Selected = sel == 1
	return &o, nil
}

func (d *DB) UpdateRenderOutput(o *models.RenderOutput) error {
	now := time.Now().UTC()
	previous := o.UpdatedAt
	sel := 0
	if o.Selected {
		sel = 1
	}
	result, err := d.conn.Exec(`
		UPDATE render_outputs
		SET video_path = ?, thumbnail_path = ?, captioned_path = ?, captions_json = ?, score = ?, qa_json = ?, selected = ?, notes = ?, updated_at = ?
		WHERE id = ? AND updated_at = ?
	`, o.VideoPath, o.ThumbnailPath, o.CaptionedPath, string(o.CaptionsJSON), o.Score, string(o.QAJSON), sel, o.Notes, now, o.ID, previous)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return fmt.Errorf("output changed; reload before saving")
	}
	o.UpdatedAt = now
	return nil
}

// --- Shots Reorder & Scene/Shot Associations ---

func (d *DB) ReorderShots(sceneID string, orderedIDs []string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for i, id := range orderedIDs {
		_, err := tx.Exec("UPDATE shots SET shot_order = ? WHERE id = ? AND scene_id = ?", i, id, sceneID)
		if err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`UPDATE scenes SET updated_at=? WHERE id=?`, time.Now().UTC(), sceneID); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) AttachShotAsset(shotID, assetID string) error {
	shot, err := d.GetShot(shotID)
	if err != nil {
		return err
	}
	assetIDs := models.IDList(shot.AssetIDsJSON)
	for _, aid := range assetIDs {
		if aid == assetID {
			return nil
		}
	}
	assetIDs = append(assetIDs, assetID)
	bytes, _ := json.Marshal(assetIDs)
	shot.AssetIDsJSON = bytes
	return d.UpdateShot(shot)
}

func (d *DB) DetachShotAsset(shotID, assetID string) error {
	shot, err := d.GetShot(shotID)
	if err != nil {
		return err
	}
	assetIDs := models.IDList(shot.AssetIDsJSON)
	filtered := make([]string, 0, len(assetIDs))
	for _, aid := range assetIDs {
		if aid != assetID {
			filtered = append(filtered, aid)
		}
	}
	bytes, _ := json.Marshal(filtered)
	shot.AssetIDsJSON = bytes
	return d.UpdateShot(shot)
}

// --- Revisions ---

func (d *DB) CreateRevision(entityType, entityID string, snapshot []byte) (*models.Revision, error) {
	id := "rev_" + uuid.New().String()[:8]
	now := time.Now().UTC()
	rev := &models.Revision{
		ID:           id,
		EntityType:   entityType,
		EntityID:     entityID,
		SnapshotJSON: snapshot,
		CreatedAt:    now,
	}
	_, err := d.conn.Exec(`
		INSERT INTO revisions (id, entity_type, entity_id, snapshot_json, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, rev.ID, rev.EntityType, rev.EntityID, string(rev.SnapshotJSON), rev.CreatedAt)
	if err != nil {
		return nil, err
	}
	return rev, nil
}

func (d *DB) ListRevisions(entityType, entityID string) ([]models.Revision, error) {
	rows, err := d.conn.Query(`
		SELECT id, entity_type, entity_id, snapshot_json, created_at
		FROM revisions
		WHERE entity_type = ? AND entity_id = ?
		ORDER BY created_at DESC
		LIMIT 20
	`, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Revision
	for rows.Next() {
		var r models.Revision
		var snap string
		if err := rows.Scan(&r.ID, &r.EntityType, &r.EntityID, &snap, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.SnapshotJSON = []byte(snap)
		list = append(list, r)
	}
	return list, nil
}

func (d *DB) GetRevision(id string) (*models.Revision, error) {
	row := d.conn.QueryRow("SELECT id, entity_type, entity_id, snapshot_json, created_at FROM revisions WHERE id = ?", id)
	var r models.Revision
	var snap string
	if err := row.Scan(&r.ID, &r.EntityType, &r.EntityID, &snap, &r.CreatedAt); err != nil {
		return nil, err
	}
	r.SnapshotJSON = []byte(snap)
	return &r, nil
}

// CreateRenderJobs commits a confirmed batch before any unit is enqueued.
func (d *DB) CreateRenderJobs(jobs []*models.RenderJob) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, j := range jobs {
		if err = insertRenderJob(tx, j); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Prompts keep internal IDs for generation; canvas labels show readable text only.
var internalIDMention = regexp.MustCompile(`\s*\((?:char|asset|scene|shot)_[0-9a-f]{6,}\)`)

func displayPrompt(prompt string) string { return internalIDMention.ReplaceAllString(prompt, "") }
