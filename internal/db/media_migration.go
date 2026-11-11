package db

import (
	"database/sql"
	"encoding/json"
)

func (d *DB) ensureMediaProviders() error {
	rows, err := d.conn.Query(`PRAGMA table_info(render_jobs)`)
	if err != nil {
		return err
	}
	hasEndpoint := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, kind string
		var fallback any
		if err = rows.Scan(&cid, &name, &kind, &notnull, &fallback, &pk); err != nil {
			rows.Close()
			return err
		}
		hasEndpoint = hasEndpoint || name == "provider_endpoint"
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if !hasEndpoint {
		if _, err = d.conn.Exec(`ALTER TABLE render_jobs ADD COLUMN provider_endpoint TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	marker, _ := d.GetAppSetting("media_provider_defaults_v1")
	if marker == "true" {
		return nil
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, kind := range []string{"image", "video"} {
		var raw string
		err = tx.QueryRow(`SELECT value FROM app_settings WHERE key=?`, kind+"_model").Scan(&raw)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if kind == "image" && raw == "" {
			err = tx.QueryRow(`SELECT value FROM app_settings WHERE key='ref_image_model'`).Scan(&raw)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
		}
		if raw != "" {
			if _, err = tx.Exec(`INSERT OR IGNORE INTO app_settings(key,value)VALUES(?,?)`, "default_"+kind+"_provider", `"openrouter"`); err != nil {
				return err
			}
		}
	}
	if _, err = tx.Exec(`INSERT INTO app_settings(key,value)VALUES('media_provider_defaults_v1','true') ON CONFLICT(key) DO UPDATE SET value='true'`); err != nil {
		return err
	}
	return tx.Commit()
}
func (d *DB) SetAppSettings(values map[string]any) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for key, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO app_settings(key,value)VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, string(raw)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// One SELECT gives a consistent route/model view while settings are switched atomically.
func (d *DB) AppSettingsSnapshot() (map[string]string, error) {
	rows, err := d.conn.Query(`SELECT key,value FROM app_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, rows.Err()
}
