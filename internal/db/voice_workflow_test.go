package db

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestVoiceDocumentRevisions(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "voice.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err = d.EnsureVoiceWorkflow(); err != nil {
		t.Fatal(err)
	}
	project, _ := d.GetActiveProjectID()
	r, err := d.SaveVoiceDocument(project, "profile", "speaker", 0, json.RawMessage(`{"voice_id":"saved"}`))
	if err != nil || r.Revision != 1 {
		t.Fatalf("save %+v %v", r, err)
	}
	if _, err = d.SaveVoiceDocument(project, "profile", "speaker", 0, json.RawMessage(`{}`)); err == nil {
		t.Fatal("accepted stale revision")
	}
	if r, err = d.SaveVoiceDocument(project, "profile", "speaker", 1, json.RawMessage(`{"voice_id":"next"}`)); err != nil || r.Revision != 2 {
		t.Fatalf("update %+v %v", r, err)
	}
	other, _ := d.CreateProject("Other", "")
	if docs, _ := d.ListVoiceDocuments(other.ID, "profile"); len(docs) != 0 {
		t.Fatal("voice profile leaked to another project")
	}
}
