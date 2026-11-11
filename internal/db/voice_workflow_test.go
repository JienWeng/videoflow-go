package db

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestVoiceWorkflowRevisionAndSubmissionFence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "voice.db")
	d, err := Open(path)
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
	task := VoiceTask{ID: "job", ProjectID: project, Fingerprint: "same", Kind: "speech", Status: "draft", Input: json.RawMessage(`{}`)}
	first, err := d.CreateVoiceTask(task)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := d.CreateVoiceTask(task)
	if err != nil || duplicate.ID != first.ID {
		t.Fatal("duplicate prepare did not recover task")
	}
	var count atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if d.ClaimVoiceTask(project, "job") == nil {
				count.Add(1)
			}
		}()
	}
	wg.Wait()
	if count.Load() != 1 {
		t.Fatalf("%d submissions", count.Load())
	}
	saved, err := d.GetVoiceTask(project, "job")
	if err != nil || saved.Status != "submitting" {
		t.Fatalf("fence lost %+v %v", saved, err)
	}
	if err = d.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	saved, err = reopened.GetVoiceTask(project, "job")
	if err != nil || saved.Status != "submitting" {
		t.Fatalf("restart lost uncertain fence %+v %v", saved, err)
	}
	if err = reopened.ClaimVoiceTask(project, "job"); err == nil {
		t.Fatal("restart permitted duplicate paid submission")
	}
	other, _ := reopened.CreateProject("Other", "")
	if _, err = reopened.GetVoiceTask(other.ID, "job"); err == nil {
		t.Fatal("cross-project task receipt leaked")
	}

}
