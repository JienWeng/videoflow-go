package db

// baseSchema is the original set of tables, created on every start if missing, in this
// order. Later changes are numbered steps in migrations.go.
var baseSchema = []string{projectTables, storyTables, renderTables, workspaceTables}

// projectTables holds the project that owns everything else.
const projectTables = `
	CREATE TABLE IF NOT EXISTS projects (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		is_active BOOLEAN DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

// storyTables hold characters, assets and the script broken into scenes and shots.
const storyTables = `
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

	CREATE TABLE IF NOT EXISTS scripts (
		id TEXT PRIMARY KEY,
		project_id TEXT,
		idea TEXT DEFAULT '',
		title TEXT DEFAULT '',
		summary TEXT DEFAULT '',
		draft_json TEXT DEFAULT '{}',
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
	`

// renderTables hold video jobs and the clips they produce.
const renderTables = `
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
		qa_json TEXT DEFAULT '{}',
		selected BOOLEAN DEFAULT 0,
		notes TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(render_job_id) REFERENCES render_jobs(id) ON DELETE CASCADE
	);
	`

// workspaceTables hold style, background operations, settings, connections and revision history.
const workspaceTables = `
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

	CREATE TABLE IF NOT EXISTS ops (
		id TEXT PRIMARY KEY,
		kind TEXT NOT NULL,
		status TEXT DEFAULT 'running',
		scene_id TEXT,
		output_id TEXT,
		project_id TEXT,
		error TEXT,
		result_json TEXT DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS app_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS provider_secrets (
		provider TEXT PRIMARY KEY,
		api_key TEXT DEFAULT '',
		base_url TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS connections (
		name TEXT PRIMARY KEY,
		label TEXT NOT NULL,
		preset TEXT NOT NULL,
		protocol TEXT NOT NULL,
		base_url TEXT,
		model TEXT DEFAULT '',
		mode TEXT DEFAULT 'auto',
		vision BOOLEAN DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS revisions (
		id TEXT PRIMARY KEY,
		entity_type TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		snapshot_json TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
