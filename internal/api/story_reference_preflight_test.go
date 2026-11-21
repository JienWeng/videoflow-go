package api

import (
	"net/http"
	"testing"
)

func TestCharacterSheetsPreflightRejectsUnsupportedSquareBeforePlanning(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/images/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"input_references":{"type":"array"},"aspect_ratio":{"type":"enum","values":["16:9"]}}}]}`))
		case "/videos/models":
			w.Write([]byte(`{"data":[{"id":"google/veo-3.1-lite","supported_durations":[1],"supported_aspect_ratios":["16:9"],"supported_frame_images":["first_frame"]}]}`))
		default:
			t.Errorf("preflight submitted inference %s", r.URL.Path)
			w.WriteHeader(500)
		}
	})
	s.directorRunner.Codex = "/bin/true"
	project, _ := s.database.GetActiveProjectID()
	s.database.CreateDirectorSession(project, "codex", "Preflight")
	response := call(t, s, "POST", "/videos/generate", `{"idea":"A puppy finds mother","create_characters":true,"scene_count":1,"target_duration":1,"aspect_ratio":"16:9"}`)
	if response.Code != 400 {
		t.Fatal("unsupported sheet format accepted", response.Code, response.Body.String())
	}
	chars, _ := s.database.ListCharacters(project)
	scenes, _ := s.database.ListScenes(project)
	ops, _ := s.database.ListOps(project, 10)
	if len(chars)+len(scenes)+len(ops) != 0 {
		t.Fatal("preflight left generated state")
	}
}
