package db

import (
	"path/filepath"
	"testing"
)

func TestDirectorSessionProjectIsolationAndInterruptedRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	project, _ := d.GetActiveProjectID()
	session, err := d.CreateDirectorSession(project, "codex", "Film")
	if err != nil {
		t.Fatal(err)
	}
	turn, err := d.BeginDirectorTurn(session.ID, project, "request-1", "chat", []byte(`{"message":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.BeginDirectorTurn(session.ID, project, "request-2", "chat", nil); err == nil {
		t.Fatal("concurrent turn allowed")
	}
	if _, err = d.GetDirectorSession(session.ID, "other"); err == nil {
		t.Fatal("cross-project session visible")
	}
	if err = d.SetDirectorNativeID(session.ID, "native-123"); err != nil {
		t.Fatal(err)
	}
	d.Close()
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	turns, err := d.ListDirectorTurns(session.ID, project)
	if err != nil || len(turns) != 1 || turns[0].Status != "interrupted" {
		t.Fatalf("%+v %v", turns, err)
	}
	if _, err = d.BeginDirectorTurn(session.ID, project, "request-1", "chat", nil); err == nil {
		t.Fatal("interrupted request replayed")
	}
	if err = d.FinishDirectorTurn(turn.ID, "succeeded", []byte(`{}`), ""); err == nil {
		t.Fatal("late completion overwrote recovery")
	}
	saved, _ := d.GetDirectorSession(session.ID, project)
	if saved.NativeID != "native-123" {
		t.Fatal("native ID lost")
	}
}
