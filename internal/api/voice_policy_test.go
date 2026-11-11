package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"videoflow-go/internal/models"
)

func TestNativeVoiceLocksSemanticAssetIdentityBeforeRun(t *testing.T) {
	var paid atomic.Int32
	s := voiceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		paid.Add(1)
		w.Write([]byte(`{"request_id":"unexpected"}`))
	})
	c := voiceCharacter(t, s)
	sample := uploadVoiceFixture(t, s, c, 10)
	call(t, s, "PUT", "/voice/profiles/"+c, fmt.Sprintf(`{"revision":0,"data":{"mode":"native_reference","sample_asset_id":%q,"approved":true}}`, sample.ID))
	call(t, s, "PUT", "/voice/scripts/main", fmt.Sprintf(`{"revision":0,"data":{"approved":true,"lines":[{"id":"u","character_id":%q,"language":"en","text":"My red CAR."}]}}`, c))
	project, _ := s.database.GetActiveProjectID()
	path := filepath.Join(s.cfg.StorageRoot, "car.png")
	os.WriteFile(path, []byte("\x89PNG\r\n\x1a\n"), 0600)
	car := &models.Asset{ProjectID: &project, Name: "CAR identity", Type: "prop", Description: "Exact RED family CAR", FilePath: path, MetadataJSON: json.RawMessage(`{"locked_traits":{"kind":"CAR","color":"RED"},"identity_note":"Keep exact emblem"}`)}
	s.database.CreateAsset(car)
	r := call(t, s, "POST", "/voice/tasks/prepare", fmt.Sprintf(`{"kind":"native_reference","script_id":"main","line_id":"u","image_asset_ids":[%q],"prompt":"Show car","duration":5}`, car.ID))
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	var task struct {
		ID, Fingerprint string
		Preview         map[string]any
	}
	json.Unmarshal(r.Body.Bytes(), &task)
	prompt, _ := task.Preview["prompt"].(string)
	for _, constraint := range []string{"CAR identity", "Exact RED family CAR", `"color":"RED"`, `"kind":"CAR"`, "Keep exact emblem"} {
		if !strings.Contains(prompt, constraint) {
			t.Errorf("prepared native prompt dropped %q: %s", constraint, prompt)
		}
	}
	car.MetadataJSON = json.RawMessage(`{"locked_traits":{"kind":"CAR","color":"BLUE"},"identity_note":"Keep exact emblem"}`)
	s.database.UpdateAsset(car)
	r = call(t, s, "POST", "/voice/tasks/"+task.ID+"/run", fmt.Sprintf(`{"confirmed":true,"fingerprint":%q}`, task.Fingerprint))
	if r.Code != 409 || paid.Load() != 0 {
		t.Fatal("semantic change reached paid provider", r.Code, r.Body.String(), paid.Load())
	}
}
func TestDirectorVoicePolicyRequiresSavedVoiceAndForcesVisualPass(t *testing.T) {
	s := voiceFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("policy made provider call") })
	s.cfg.UseLocalText = true
	c := voiceCharacter(t, s)
	project, _ := s.database.GetActiveProjectID()
	ctx := context.WithValue(context.Background(), directorBindingKey{}, directorBinding{ProjectID: project, Selection: directorSelection{CharacterIDs: []string{c}}})
	job := func() *models.RenderJob {
		return &models.RenderJob{ProjectID: &project, RequestJSON: json.RawMessage(`{"prompt":"Speaker says hello","generate_audio":true}`)}
	}
	if err := s.applyDirectorVoicePolicy(ctx, job()); err == nil {
		t.Fatal("missing saved voice accepted")
	}
	call(t, s, "PUT", "/voice/profiles/"+c, `{"revision":0,"data":{"voice_id":"ExactVoiceID","approved":true}}`)
	scene := &models.Scene{ProjectID: &project, Title: "Keep scene title", Summary: "Keep scene action", CharacterIDsJSON: voiceJSON([]string{c}), SceneJSON: voiceJSON(map[string]any{"generation_brief": map[string]any{"conversation_mode": "dialogue"}})}
	s.database.CreateScene(scene)
	sceneJob := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID}
	if err := s.applyDirectorVoicePolicy(ctx, sceneJob); err != nil || !strings.Contains(string(sceneJob.RequestJSON), "Keep scene action") {
		t.Fatal("voice policy dropped deferred scene prompt", err, string(sceneJob.RequestJSON))
	}
	ready := job()
	if err := s.applyDirectorVoicePolicy(ctx, ready); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	json.Unmarshal(ready.RequestJSON, &payload)
	if payload["generate_audio"] != false || !strings.Contains(payload["prompt"].(string), "ExactVoiceID") || !strings.Contains(string(ready.RequestJSON), "voice_policy") {
		t.Fatal(string(ready.RequestJSON))
	}
	call(t, s, "PUT", "/voice/profiles/"+c, `{"revision":1,"data":{"voice_id":"ExactVoiceID","approved":false}}`)
	if err := s.applyDirectorVoicePolicy(ctx, job()); err == nil {
		t.Fatal("unapproved voice accepted")
	}
	silent := job()
	silent.RequestJSON = json.RawMessage(`{"prompt":"Silent visual","conversation_mode":"silent","generate_audio":true}`)
	if err := s.applyDirectorVoicePolicy(ctx, silent); err != nil {
		t.Fatal(err)
	}
	s.cfg.UseLocalText = false
	if err := s.applyDirectorVoicePolicy(ctx, job()); err != nil {
		t.Fatal("legacy policy should skip", err)
	}
}
func TestDirectorNativeVoicePolicyRejectsOrdinaryVideo(t *testing.T) {
	s := voiceFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("policy made provider call") })
	s.cfg.UseLocalText = true
	c := voiceCharacter(t, s)
	sample := uploadVoiceFixture(t, s, c, 10)
	project, _ := s.database.GetActiveProjectID()
	call(t, s, "PUT", "/voice/profiles/"+c, fmt.Sprintf(`{"revision":0,"data":{"mode":"native_reference","sample_asset_id":%q,"approved":true}}`, sample.ID))
	scene := &models.Scene{ProjectID: &project, Title: "Dialogue", CharacterIDsJSON: voiceJSON([]string{c}), SceneJSON: voiceJSON(map[string]any{"generation_brief": map[string]any{"conversation_mode": "dialogue"}, "spoken_lines": []SpokenLine{{ID: "u", CharacterID: c, Text: "Hello", Language: "en"}}})}
	s.database.CreateScene(scene)
	job := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, RequestJSON: json.RawMessage(`{"prompt":"Hello","generate_audio":true}`)}
	if err := s.applyDirectorVoicePolicy(context.Background(), job); err == nil || !strings.Contains(err.Error(), "/voices") {
		t.Fatal("native sample silently dropped", err)
	}
}

func TestNativeVoiceRejectsPendingGeneratedReferenceBeforePreparation(t *testing.T) {
	var paid atomic.Int32
	s := voiceFixture(t, func(w http.ResponseWriter, r *http.Request) { paid.Add(1) })
	c := voiceCharacter(t, s)
	sample := uploadVoiceFixture(t, s, c, 10)
	project, _ := s.database.GetActiveProjectID()
	call(t, s, "PUT", "/voice/profiles/"+c, fmt.Sprintf(`{"revision":0,"data":{"mode":"native_reference","sample_asset_id":%q,"approved":true}}`, sample.ID))
	call(t, s, "PUT", "/voice/scripts/main", fmt.Sprintf(`{"revision":0,"data":{"approved":true,"lines":[{"id":"u","character_id":%q,"language":"en","text":"Hello."}]}}`, c))
	path := filepath.Join(s.cfg.StorageRoot, "pending.png")
	os.WriteFile(path, []byte("\x89PNG\r\n\x1a\n"), 0600)
	image := &models.Asset{ProjectID: &project, Name: "Pending generated reference", Type: "storyboard", FilePath: path, MetadataJSON: json.RawMessage(`{"consistency_review":"pending"}`)}
	s.database.CreateAsset(image)
	body := fmt.Sprintf(`{"kind":"native_reference","script_id":"main","line_id":"u","image_asset_ids":[%q],"prompt":"Hello","duration":5}`, image.ID)
	r := call(t, s, "POST", "/voice/tasks/prepare", body)
	if r.Code != 400 || paid.Load() != 0 {
		t.Fatal("unreviewed image accepted", r.Code, r.Body.String(), paid.Load())
	}
	image.MetadataJSON = json.RawMessage(`{"consistency_review":"approved"}`)
	s.database.UpdateAsset(image)
	r = call(t, s, "POST", "/voice/tasks/prepare", body)
	if r.Code != 200 || paid.Load() != 0 {
		t.Fatal("approved image rejected", r.Code, r.Body.String(), paid.Load())
	}
	var task struct{ ID, Fingerprint string }
	json.Unmarshal(r.Body.Bytes(), &task)
	image.MetadataJSON = json.RawMessage(`{"consistency_review":"rejected"}`)
	s.database.UpdateAsset(image)
	r = call(t, s, "POST", "/voice/tasks/"+task.ID+"/run", fmt.Sprintf(`{"confirmed":true,"fingerprint":%q}`, task.Fingerprint))
	if r.Code != 409 || paid.Load() != 0 {
		t.Fatal("review change failed to invalidate prepared action", r.Code, r.Body.String(), paid.Load())
	}
}
func TestOrdinaryUnboundVisualPassDoesNotInventVoice(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no provider") })
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	job := &models.RenderJob{ProjectID: &project, RequestJSON: []byte(`{"prompt":"A landscape"}`)}
	if err := s.applyDirectorVoicePolicy(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	json.Unmarshal(job.RequestJSON, &payload)
	if payload["generate_audio"] != false {
		t.Fatal("unbound render can invent a generic voice")
	}
}
