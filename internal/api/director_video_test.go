package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"videoflow-go/internal/models"
)

func TestOrdinaryRenderCapturesIdentityAndRechecksBeforeSubmission(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no paid request") })
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	path := filepath.Join(s.cfg.StorageRoot, "car.png")
	os.WriteFile(path, []byte("reference"), 0600)
	a := &models.Asset{ProjectID: &project, Name: "Red car", Type: "prop", FilePath: path, MetadataJSON: []byte(`{"locked_traits":{"kind":"car","color":"red"}}`)}
	s.database.CreateAsset(a)
	ctx := context.WithValue(context.Background(), directorBindingKey{}, directorBinding{ProjectID: project, Selection: directorSelection{AssetIDs: []string{a.ID}}})
	job := &models.RenderJob{ProjectID: &project, RequestJSON: []byte(`{"prompt":"Animate car","frame_images":[{"image_url":"unrelated"}]}`)}
	if err := s.freezeDirectorVideo(ctx, job); err == nil {
		t.Fatal("conflicting manual image override accepted")
	}
	// No image bytes need to be sent to prove the durable constraint fence.
	snapshot, _ := s.buildDirectorContext(project, directorSelection{AssetIDs: []string{a.ID}})
	job.RequestJSON = voiceJSON(map[string]any{"prompt": "Animate red car", "director_context": snapshot})
	if err := s.validateFrozenVideo(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	a.Description = "Changed identity"
	s.database.UpdateAsset(a)
	if err := s.validateFrozenVideo(context.Background(), job); err == nil {
		t.Fatal("queued render ignored identity metadata change")
	}
	if !strings.Contains(string(job.RequestJSON), `"color":"red"`) {
		t.Fatal("locked attributes not captured")
	}
}
func TestFrozenVideoRejectsUnreviewedReference(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no provider") })
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	path := filepath.Join(s.cfg.StorageRoot, "pending.png")
	os.WriteFile(path, []byte("pending"), 0600)
	a := &models.Asset{ProjectID: &project, Type: "storyboard", FilePath: path, MetadataJSON: []byte(`{"consistency_review":"pending"}`)}
	s.database.CreateAsset(a)
	ctx := context.WithValue(context.Background(), directorBindingKey{}, directorBinding{ProjectID: project, Selection: directorSelection{AssetIDs: []string{a.ID}}})
	job := &models.RenderJob{ProjectID: &project, RequestJSON: json.RawMessage(`{"prompt":"not approved"}`)}
	if err := s.freezeDirectorVideo(ctx, job); err == nil || !strings.Contains(err.Error(), "review") {
		t.Fatal("unreviewed generated ref accepted", err)
	}
}
func TestExplicitRetryRetainsFrozenFrameAndPolicy(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no paid call") })
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	path := filepath.Join(s.cfg.StorageRoot, "approved.png")
	os.WriteFile(path, []byte("source bytes"), 0600)
	a := &models.Asset{ProjectID: &project, Type: "prop", FilePath: path}
	s.database.CreateAsset(a)
	session, _ := s.database.CreateDirectorSession(project, "codex", "Retry")
	selection := directorSelection{AssetIDs: []string{a.ID}}
	s.database.SetDirectorSelection(session.ID, project, voiceJSON(selection))
	snapshot, _ := s.buildDirectorContext(project, selection)
	job := &models.RenderJob{ProjectID: &project, Provider: "openrouter", Model: "google/veo-3.1-lite", Status: "failed", RequestJSON: voiceJSON(map[string]any{"prompt": "Frozen car", "director_context": snapshot, "director_keyframe": a.ID, "frame_images": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,frozen"}, "frame_type": "first_frame"}}, "generate_audio": false})}
	s.database.CreateRenderJob(job)
	result := call(t, s, "POST", "/render-jobs/"+job.ID+"/resubmit", `{}`)
	if result.Code != 202 {
		t.Fatalf("frozen retry rejected: %d %s", result.Code, result.Body.String())
	}
}
func TestFrozenContextRetainsExplicitSelectionWhenCharactersExpandReferences(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no provider") })
	project, _ := s.database.GetActiveProjectID()
	character := &models.Character{ProjectID: &project, Name: "Speaker"}
	s.database.CreateCharacter(character)
	photoPath := filepath.Join(s.cfg.StorageRoot, "photo.png")
	os.WriteFile(photoPath, []byte("photo"), 0600)
	photo := &models.Asset{ProjectID: &project, CharacterID: &character.ID, Type: "character_reference", FilePath: photoPath}
	s.database.CreateUploadedAsset(photo)
	selection := directorSelection{CharacterIDs: []string{character.ID}}
	for i := 0; i < 32; i++ {
		p := filepath.Join(s.cfg.StorageRoot, fmt.Sprintf("sample%d.wav", i))
		os.WriteFile(p, []byte("sample"), 0600)
		a := &models.Asset{ProjectID: &project, Type: "audio", FilePath: p}
		s.database.CreateAsset(a)
		selection.AssetIDs = append(selection.AssetIDs, a.ID)
	}
	snapshot, err := s.buildDirectorContext(project, selection)
	if err != nil || len(snapshot.Assets) != 33 {
		t.Fatal(err)
	}
	job := &models.RenderJob{ProjectID: &project, RequestJSON: voiceJSON(map[string]any{"director_context": snapshot})}
	if err = s.validateFrozenVideo(context.Background(), job); err != nil {
		t.Fatal("valid expanded context cannot be revalidated", err)
	}
}
