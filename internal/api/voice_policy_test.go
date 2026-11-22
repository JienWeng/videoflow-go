package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"videoflow-go/internal/models"
)

// Scenes saved by earlier versions asked for "saved" voices added after the clip.
// Such a request is now made silent, and a speaker without a saved voice blocks nothing.
func TestSavedAudioModeRendersSilentWithoutSavedVoices(t *testing.T) {
	s := voiceFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("policy made provider call") })
	c := voiceCharacter(t, s)
	project, _ := s.database.GetActiveProjectID()
	scene := &models.Scene{ProjectID: &project, Title: "Keep scene title", Summary: "Keep scene action", CharacterIDsJSON: voiceJSON([]string{c}), SceneJSON: voiceJSON(map[string]any{
		"generation_brief": map[string]any{"conversation_mode": "dialogue"},
		"production":       map[string]any{"audio_mode": "saved", "strategy": "auto"},
		"spoken_lines":     []SpokenLine{{ID: "u", CharacterID: c, Text: "Hello", Language: "en"}},
	})}
	s.database.CreateScene(scene)
	if mode := sceneProduction(scene).AudioMode; mode != "silent" {
		t.Fatal("stored saved mode not read as silent", mode)
	}
	ctx := context.WithValue(context.Background(), directorBindingKey{}, directorBinding{ProjectID: project, Selection: directorSelection{CharacterIDs: []string{c}}})
	for name, job := range map[string]*models.RenderJob{
		"request asks for saved": {ProjectID: &project, RequestJSON: json.RawMessage(`{"prompt":"Speaker says hello","audio_mode":"saved","generate_audio":true}`)},
		"scene stores saved":     {ProjectID: &project, SceneID: &scene.ID},
	} {
		if err := s.applyDirectorVoicePolicy(ctx, job); err != nil {
			t.Fatal(name, "blocked by a missing saved voice:", err)
		}
		var payload map[string]any
		json.Unmarshal(job.RequestJSON, &payload)
		if payload["audio_mode"] != "silent" || payload["generate_audio"] != false {
			t.Fatal(name, string(job.RequestJSON))
		}
		if strings.Contains(string(job.RequestJSON), "/voices") {
			t.Fatal(name, "still points at the removed voice stage", string(job.RequestJSON))
		}
	}
	sceneJob := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID}
	// A scene request saved with no text of its own gets the scene's before policy text is added.
	if err := s.freezeDirectorVideo(ctx, sceneJob); err != nil || !strings.Contains(string(sceneJob.RequestJSON), "Keep scene action") || !strings.Contains(string(sceneJob.RequestJSON), "SILENT visual pass") {
		t.Fatal("voice policy dropped deferred scene prompt", err, string(sceneJob.RequestJSON))
	}
	unknown := &models.RenderJob{ProjectID: &project, RequestJSON: json.RawMessage(`{"prompt":"x","audio_mode":"dubbed"}`)}
	if err := s.applyDirectorVoicePolicy(ctx, unknown); err == nil {
		t.Fatal("unknown audio mode accepted")
	}
}

func TestOrdinaryUnboundVisualPassDoesNotInventVoice(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no provider") })
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
