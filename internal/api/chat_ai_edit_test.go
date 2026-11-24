package api

import (
	"net/http"
	"strings"
	"testing"

	"videoflow-go/internal/agents"
	"videoflow-go/internal/models"
)

func TestChatAIEditNeedsEveryVideoOfTheEpisode(t *testing.T) {
	s, _ := bareServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no remote call") })
	if canonicalChatAction(aiEditAction) != aiEditAction || !selectedChatActionAllowed("scene", aiEditAction) {
		t.Fatal("ai_edit is not a supported project-level action")
	}
	project, _ := s.database.GetActiveProjectID()
	script := "script_1"
	scene := models.Scene{ID: "scene_1", ProjectID: &project, ScriptID: &script, Title: "Opening"}
	turn := &chatTurn{projectID: project, scenes: []models.Scene{scene}}
	intent := map[string]any{"action": aiEditAction}
	if got := s.resolveChatAIEdit(turn, &agents.ChatIntentResponse{SceneID: "scene_1"}, aiEditAction, intent); got != "unknown" || !strings.Contains(intent["reply"].(string), "“Opening” still need a video") {
		t.Fatalf("%s %v", got, intent)
	}
	turn.jobs = []models.RenderJob{{ID: "job_1", ProjectID: &project, SceneID: &scene.ID, Status: models.StatusSucceeded, RequestJSON: []byte(`{}`)}}
	intent = map[string]any{"action": aiEditAction}
	if got := s.resolveChatAIEdit(turn, &agents.ChatIntentResponse{SceneID: "scene_1"}, aiEditAction, intent); got != aiEditAction || intent["script_id"] != script {
		t.Fatalf("%s %v", got, intent)
	}
}
