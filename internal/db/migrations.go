package db

import (
	"database/sql"
	"fmt"
)

// Schema changes made after the tables above were first created are numbered steps here.
// The database records the last step it has run (SQLite's user_version), so each runs
// once, in order, in its own transaction. To change the schema, append a step; never
// edit or reorder one that has shipped. The ensure* functions that run on every start
// are idempotent repairs and table creation for databases older than this list.
var migrations = []func(tx *sql.Tx) error{
	// 1: per-agent model settings were removed; the table is no longer read or written.
	func(tx *sql.Tx) error {
		_, err := tx.Exec(`DROP TABLE IF EXISTS agent_settings`)
		return err
	},
}

func (d *DB) runMigrations() error {
	var version int
	if err := d.conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("this database is from a newer version of the app (schema %d, this app knows %d)", version, len(migrations))
	}
	for i := version; i < len(migrations); i++ {
		tx, err := d.conn.Begin()
		if err != nil {
			return err
		}
		if err = migrations[i](tx); err == nil {
			// PRAGMA does not take bound values; i+1 is an integer we control.
			_, err = tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1))
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("step %d: %w", i+1, err)
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
