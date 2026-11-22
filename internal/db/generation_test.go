package db

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"videoflow-go/internal/models"
)

func TestStorySnapshotFailureRollsBackCharactersAndStory(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	project, _ := d.GetActiveProjectID()
	script := &models.Script{ProjectID: &project, Title: "Dog story"}
	characters := []models.Character{{ID: "new-pup", ProjectID: &project, Name: "Pip"}}
	scenes := []models.Scene{{ID: "duplicate", Title: "One", CharacterIDsJSON: []byte(`["new-pup"]`)}, {ID: "duplicate", Title: "Two"}}
	if err = d.SaveStorySnapshot(script, scenes, characters); err == nil {
		t.Fatal("invalid snapshot committed")
	}
	chars, _ := d.ListCharacters(project)
	scripts, _ := d.ListScripts(project)
	stored, _ := d.ListScenes(project)
	if len(chars)+len(scripts)+len(stored) != 0 {
		t.Fatal("partial story or characters left behind")
	}
}

func TestScriptSceneFailureRollsBackWholeSnapshot(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	project, _ := d.GetActiveProjectID()
	script := &models.Script{ProjectID: &project, Title: "Test"}
	scenes := []models.Scene{{ID: "duplicate", Title: "A"}, {ID: "duplicate", Title: "B"}}
	if err = d.SaveScriptScenes(script, scenes); err == nil {
		t.Fatal("bad snapshot committed")
	}
	saved, _ := d.ListScripts(project)
	stored, _ := d.ListScenes(project)
	if len(saved) != 0 || len(stored) != 0 {
		t.Fatal("partial snapshot left behind")
	}
}
func TestShotReplacementFailurePreservesApprovedShots(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	project, _ := d.GetActiveProjectID()
	scene := &models.Scene{ProjectID: &project, Title: "A", CharacterIDsJSON: []byte("[]"), SceneJSON: []byte("{}")}
	d.CreateScene(scene)
	old := &models.Shot{SceneID: scene.ID, Prompt: "approved"}
	d.CreateShot(old)
	shots := []models.Shot{{ID: "duplicate"}, {ID: "duplicate"}}
	if err = d.ReplaceSceneShots(scene, shots); err == nil {
		t.Fatal("invalid replacement committed")
	}
	saved, _ := d.ListShots(scene.ID)
	if len(saved) != 1 || saved[0].ID != old.ID {
		t.Fatal("approved shots lost")
	}
}

func TestShotEditFencesLateRegeneration(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	project, _ := d.GetActiveProjectID()
	scene := &models.Scene{ProjectID: &project, Title: "Scene", CharacterIDsJSON: []byte("[]"), SceneJSON: []byte("{}")}
	d.CreateScene(scene)
	shot := &models.Shot{SceneID: scene.ID, Prompt: "Old"}
	d.CreateShot(shot)
	snapshot, _ := d.GetScene(scene.ID)
	shot.Prompt = "New approved edit"
	if err = d.UpdateShot(shot); err != nil {
		t.Fatal(err)
	}
	if err = d.ReplaceSceneShots(snapshot, []models.Shot{{Prompt: "Late AI result"}}); err == nil {
		t.Fatal("late regeneration deleted the approved edit")
	}
}
func TestStaleOutputCannotOverwriteNewCaptions(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	job := &models.RenderJob{Provider: "local", Model: "fixture", Status: "succeeded"}
	d.CreateRenderJob(job)
	output := &models.RenderOutput{RenderJobID: job.ID, VideoPath: "storage/raw.mp4"}
	d.CreateRenderOutput(output)
	old, _ := d.GetRenderOutput(output.ID)
	fresh, _ := d.GetRenderOutput(output.ID)
	fresh.CaptionsJSON = []byte(`{"segments":[],"style":"clean"}`)
	d.UpdateRenderOutput(fresh)
	old.CaptionsJSON = []byte(`{"segments":[{"start":0,"end":1,"text":"Stale"}],"style":"clean"}`)
	if err = d.UpdateRenderOutput(old); err == nil {
		t.Fatal("late caption result overwrote a newer save")
	}
}
func TestInterruptedAcceptedOperationKeepsRecoveryFence(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	op := &models.Op{Kind: "storyboard", Status: "running"}
	d.CreateOp(op)
	d.BeginRequest("old-key", "same-input")
	body := []byte(`{"op_id":"` + op.ID + `"}`)
	d.FinishRequest("old-key", 202, body)
	d.InterruptOperations()
	replay, err := d.BeginRequest("new-key", "same-input")
	if err != nil || replay == nil {
		t.Fatal("interrupted accepted paid operation can be repeated without review", err)
	}
	requests, _ := d.PendingRequests()
	if len(requests) != 1 {
		t.Fatal("interrupted accepted request is absent from recovery")
	}
}

func TestRecoveryAcknowledgementRequiresInterruptedOperation(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	op := &models.Op{Kind: "storyboard", Status: "running"}
	d.CreateOp(op)
	d.BeginRequest("key", "fingerprint")
	d.FinishRequest("key", 202, []byte(`{"op_id":"`+op.ID+`"}`))
	if err = d.AcknowledgeRequest("key", "Reviewed provider activity"); err == nil {
		t.Fatal("active operation was unlocked")
	}
	d.InterruptOperations()
	if err = d.AcknowledgeRequest("key", "Reviewed provider activity"); err != nil {
		t.Fatal(err)
	}
	replay, err := d.BeginRequest("new-key", "fingerprint")
	if err != nil || replay != nil {
		t.Fatal("reviewed operation did not unlock", err)
	}
}
func TestEndpointPinPreservesSubmissionFence(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	job := &models.RenderJob{Provider: "fal", Model: "model", Status: "pending"}
	d.CreateRenderJob(job)
	claimed, err := d.ClaimRenderSubmission(job.ID, job.Model, []byte(`{}`))
	if err != nil || !claimed {
		t.Fatal(claimed, err)
	}
	endpoint, err := d.PinRenderEndpoint(job.ID, "https://queue.fal.run")
	if err != nil || endpoint != "https://queue.fal.run" {
		t.Fatal(endpoint, err)
	}
	saved, _ := d.GetRenderJob(job.ID)
	if saved.Stage == nil || *saved.Stage != "submitting" {
		t.Fatal("pin reset submission fence")
	}
	endpoint, err = d.PinRenderEndpoint(job.ID, "https://different.invalid")
	if err != nil || endpoint != "https://queue.fal.run" {
		t.Fatal("endpoint was redirected", endpoint, err)
	}
}

func TestStoryContinuationHasOneAtomicOwner(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "claims.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	project, _ := d.GetActiveProjectID()
	op := &models.Op{ProjectID: &project, Kind: "video_generation", Status: "succeeded", ResultJSON: []byte(`{"status":"awaiting_character_review"}`)}
	if err = d.CreateOp(op); err != nil {
		t.Fatal(err)
	}
	if claimed, err := d.ClaimStoryContinuation(op.ID, "wrong-project", op.ResultJSON); err != nil || claimed {
		t.Fatal("cross-project claim", claimed, err)
	}
	if claimed, err := d.ClaimStoryContinuation(op.ID, project, []byte(`{}`)); err != nil || claimed {
		t.Fatal("changed snapshot claim", claimed, err)
	}
	var wg sync.WaitGroup
	var claimed atomic.Int32
	for range 8 {
		wg.Go(func() {
			ok, err := d.ClaimStoryContinuation(op.ID, project, op.ResultJSON)
			if err != nil {
				t.Error(err)
			}
			if ok {
				claimed.Add(1)
			}
		})
	}
	wg.Wait()
	if claimed.Load() != 1 {
		t.Fatal("multiple continuation owners", claimed.Load())
	}
}
