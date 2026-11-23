package db

import (
	"errors"
	"fmt"
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

// Step 2 adds the decision log to a database already at step 1, and a project's
// decisions go with the project.
func TestDecisionLogMigrationAndProjectCascade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.conn.Exec(`DROP TABLE project_decisions; PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	d.Close()
	if d, err = Open(path); err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var version int
	d.conn.QueryRow(`PRAGMA user_version`).Scan(&version)
	if version != 2 {
		t.Fatalf("version %d after step 2", version)
	}
	project, err := d.GetActiveProjectID()
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := d.SetDecision(project, "aspect_ratio", "9:16", DecisionByUser, false); !ok || err != nil {
		t.Fatal(ok, err)
	}
	// The assistant may not overrule the user, but the user may change their mind.
	if ok, err := d.SetDecision(project, "aspect_ratio", "16:9", DecisionByAssistant, true); ok || err != nil {
		t.Fatal("assistant replaced a user decision", ok, err)
	}
	if ok, err := d.SetDecision(project, "aspect_ratio", "1:1", DecisionByUser, false); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if _, err = d.SetDecision(project, "music", "piano", "someone", false); err == nil {
		t.Fatal("unknown source accepted")
	}
	if _, err = d.conn.Exec(`INSERT INTO project_decisions(project_id,key,value,source,created_at) VALUES(?,?,?,?,?)`, project, "x", "y", "robot", "now"); err == nil {
		t.Fatal("the table accepts an unknown source")
	}
	got, err := d.GetDecision(project, "aspect_ratio")
	if err != nil || got.Value != "1:1" || got.Source != DecisionByUser || got.CreatedAt.IsZero() {
		t.Fatalf("%+v %v", got, err)
	}
	if removed, err := d.DeleteDecision(project, "aspect_ratio"); !removed || err != nil {
		t.Fatal(removed, err)
	}
	if removed, _ := d.DeleteDecision(project, "aspect_ratio"); removed {
		t.Fatal("deleted twice")
	}
	d.SetDecision(project, "language", "en", DecisionByUser, false)
	if _, err = d.conn.Exec(`DELETE FROM projects WHERE id=?`, project); err != nil {
		t.Fatal(err)
	}
	var left int
	d.conn.QueryRow(`SELECT count(*) FROM project_decisions`).Scan(&left)
	if left != 0 {
		t.Fatal("decisions outlived their project")
	}
}

func TestDecisionLogIsCapped(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	project, _ := d.GetActiveProjectID()
	for i := 0; i < MaxProjectDecisions; i++ {
		if _, err = d.SetDecision(project, fmt.Sprintf("notes:%d", i), "x", DecisionByUser, false); err != nil {
			t.Fatal(i, err)
		}
	}
	if _, err = d.SetDecision(project, "one_more", "x", DecisionByUser, false); !errors.Is(err, ErrDecisionLogFull) {
		t.Fatal(err)
	}
	// Changing a key already kept is still allowed.
	if ok, err := d.SetDecision(project, "notes:0", "y", DecisionByUser, false); !ok || err != nil {
		t.Fatal(ok, err)
	}
	list, _ := d.ListDecisions(project)
	if len(list) != MaxProjectDecisions || list[0].Key != "notes:0" {
		t.Fatalf("%d, newest %s", len(list), list[0].Key)
	}
}
