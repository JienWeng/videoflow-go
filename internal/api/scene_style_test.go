package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"videoflow-go/internal/models"
)

func TestSceneLookOverridesProjectWithoutChangingOtherSceneInputs(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("saving style called a provider: %s", r.URL.Path)
		http.Error(w, "unexpected provider call", 500)
	})
	project, _ := s.database.GetActiveProjectID()
	if err := s.database.UpsertStyleGuide(&models.StyleGuide{ProjectID: &project, StylePrompt: "cartoon", Palette: "pastels"}); err != nil {
		t.Fatal(err)
	}
	scene := &models.Scene{ProjectID: &project, Title: "KLCC", SceneJSON: json.RawMessage(`{"generation_brief":{"idea":"Tour KLCC","style":"animated","conversation_mode":"narration"},"production":{"mode":"native"},"spoken_lines":[]}`)}
	if err := s.database.CreateScene(scene); err != nil {
		t.Fatal(err)
	}
	before, err := s.sceneInputRevision(scene)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"preset_id": "live-action-drama", "expected_updated_at": scene.UpdatedAt})
	path := "/scenes/" + scene.ID + "/style"
	if response := call(t, s, "PUT", path, string(body)); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	updated, err := s.database.GetScene(scene.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.sceneInputRevision(updated)
	if err != nil || after == before {
		t.Fatalf("storyboard revision did not change: %v", err)
	}
	brief := sceneBrief(updated)
	if brief.CreativePreset == nil || !strings.Contains(s.sceneVisualStyle(updated), "photorealistic") || brief.Idea != "Tour KLCC" || brief.ConversationMode != "narration" || !strings.Contains(string(updated.SceneJSON), `"production":{"mode":"native"}`) {
		t.Fatalf("unexpected scene snapshot: %s", updated.SceneJSON)
	}
	if response := call(t, s, "PUT", path, string(body)); response.Code != 409 {
		t.Fatalf("stale style update: %d", response.Code)
	}
	body, _ = json.Marshal(map[string]any{"preset_id": "custom", "prompt": "Practical creature effects in tropical daylight", "expected_updated_at": updated.UpdatedAt})
	if response := call(t, s, "PUT", path, string(body)); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	updated, err = s.database.GetScene(scene.ID)
	if err != nil {
		t.Fatal(err)
	}
	if brief := sceneBrief(updated); brief.CreativePreset != nil || brief.Style != "Practical creature effects in tropical daylight" {
		t.Fatalf("old preset leaked into custom style: %+v", brief)
	}
}
