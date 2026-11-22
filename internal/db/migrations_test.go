package db

import (
	"path/filepath"
	"testing"
)

// Steps run once, are recorded, and a database from a newer app is refused.
func TestMigrationsRunOnceAndAreRecorded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.conn.Exec(`CREATE TABLE agent_settings(agent TEXT)`); err != nil {
		t.Fatal(err)
	}
	var version int
	d.conn.QueryRow(`PRAGMA user_version`).Scan(&version)
	if version != len(migrations) {
		t.Fatalf("version %d, want %d", version, len(migrations))
	}
	d.Close()
	// Reopening does not run step 1 again, so the table made by hand survives.
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	d.conn.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='agent_settings'`).Scan(&n)
	if n != 1 {
		t.Fatal("a step ran twice")
	}
	if _, err = d.conn.Exec(`PRAGMA user_version = 999`); err != nil {
		t.Fatal(err)
	}
	d.Close()
	if _, err = Open(path); err == nil {
		t.Fatal("a database from a newer app was opened")
	}
}
