package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"videoflow-go/internal/composition"
	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
)

func TestCompositionCaptionImportPreservesSourceAndRejectsEdits(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected provider") })
	project, _ := s.database.GetActiveProjectID()
	s.database.EnsureVoiceWorkflow()
	doc, e := s.database.SaveVoiceDocument(project, "script", "spoken", 0, json.RawMessage(`{"approved":true,"lines":[]}`))
	if e != nil {
		t.Fatal(e)
	}
	os.MkdirAll(filepath.Join(s.cfg.StorageRoot, "outputs"), 0700)
	os.WriteFile(filepath.Join(s.cfg.StorageRoot, "outputs", "source.mp4"), []byte("fixture"), 0600)
	raw, _ := json.Marshal(map[string]any{"source": "approved_script", "reviewed": true, "source_bindings": []db.VoiceBinding{{Kind: "script", ID: doc.ID, Revision: doc.Revision}}, "segments": []models.CaptionSegment{{ID: "spoken-1", CharacterID: "speaker", Language: "en", Start: .5, End: 1.5, Text: "Approved words"}}})
	job := &models.RenderJob{ProjectID: &project, Status: "succeeded", Provider: "local", Model: "fixture", RequestJSON: []byte("{}")}
	output := &models.RenderOutput{VideoPath: "storage/outputs/source.mp4", CaptionsJSON: raw}
	if e = s.database.SaveLocalOutput(job, output); e != nil {
		t.Fatal(e)
	}
	clip := composition.Clip{ID: "clip-1", AssetID: "output-" + output.ID, Start: 2, Duration: 2, Speed: 1}
	cues, binding, e := s.importCompositionCaptions(project, clip)
	if e != nil {
		t.Fatal(e)
	}
	if len(cues) != 1 || cues[0].Start != 2.5 || cues[0].UtteranceID != "spoken-1" || cues[0].CharacterID != "speaker" || cues[0].Language != "en" {
		t.Fatalf("lost aligned fields: %+v", cues)
	}
	m := composition.Manifest{Version: 1, Width: 640, Height: 360, FPS: 24, Duration: 4, Clips: []composition.Clip{clip}, Captions: cues, CaptionBindings: []composition.CaptionBinding{binding}}
	if e = s.validateCompositionCaptions(project, m); e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	revision, err := composition.Save(root, project, m, map[string]string{clip.AssetID: filepath.Join(s.cfg.StorageRoot, "outputs", "source.mp4")})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := composition.Load(root, project, revision)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.validateCompositionCaptions(project, loaded); err != nil {
		t.Fatal("saved source binding changed", err)
	}
	os.WriteFile(filepath.Join(s.cfg.StorageRoot, "outputs", "source.mp4"), []byte("replacement"), 0600)
	if s.validateCompositionCaptions(project, m) == nil {
		t.Fatal("replaced source media accepted")
	}
	os.WriteFile(filepath.Join(s.cfg.StorageRoot, "outputs", "source.mp4"), []byte("fixture"), 0600)
	original := output.CaptionsJSON
	output.CaptionsJSON = json.RawMessage(strings.Replace(string(original), "Approved words", "Changed source", 1))
	if err = s.database.UpdateRenderOutput(output); err != nil {
		t.Fatal(err)
	}
	if s.validateCompositionCaptions(project, m) == nil {
		t.Fatal("edited source captions accepted")
	}
	output.CaptionsJSON = original
	s.database.UpdateRenderOutput(output)
	m.Captions[0].Text = "Invented words"
	if s.validateCompositionCaptions(project, m) == nil {
		t.Fatal("approved text silently changed")
	}
	m.Captions = cues
	m.Captions[0].Text = "Approved words"
	m.Clips[0].Trim = .1
	if s.validateCompositionCaptions(project, m) == nil {
		t.Fatal("stale trim accepted")
	}
	m.Clips[0].Trim = 0
	s.database.SaveVoiceDocument(project, "script", "spoken", doc.Revision, json.RawMessage(`{"approved":true,"lines":[]}`))
	if s.validateCompositionCaptions(project, m) == nil {
		t.Fatal("stale script accepted")
	}
	other, _ := s.database.CreateProject("Other", "")
	if _, _, e = s.importCompositionCaptions(other.ID, clip); e == nil {
		t.Fatal("cross-project captions accepted")
	}
}
