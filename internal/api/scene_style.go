package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleSceneStyle(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	scene, sceneErr := s.database.GetScene(chi.URLParam(r, "id"))
	if err != nil || sceneErr != nil || scene.ProjectID == nil || *scene.ProjectID != project {
		http.Error(w, "scene not found in active project", http.StatusNotFound)
		return
	}
	var body struct {
		PresetID          string    `json:"preset_id"`
		Prompt            string    `json:"prompt"`
		ExpectedUpdatedAt time.Time `json:"expected_updated_at"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil {
		http.Error(w, "invalid scene look", http.StatusBadRequest)
		return
	}
	if !scene.UpdatedAt.Equal(body.ExpectedUpdatedAt) {
		http.Error(w, "scene changed; reload before editing its look", http.StatusConflict)
		return
	}
	brief := sceneBrief(scene)
	brief.CreativePreset = nil
	brief.PresetID = body.PresetID
	brief.StyleFingerprint = ""
	switch body.PresetID {
	case "custom":
		brief.Style = strings.TrimSpace(body.Prompt)
		if brief.Style == "" || len(brief.Style) > 5000 {
			http.Error(w, "enter a visual direction between 1 and 5000 bytes", http.StatusBadRequest)
			return
		}
	case "project-style":
		brief.Style = s.stylePrompt(&project)
	default:
		preset, e := s.findPreset(body.PresetID)
		if e != nil {
			http.Error(w, e.Error(), http.StatusBadRequest)
			return
		}
		brief.CreativePreset = preset
		brief.Style = preset.Style
	}
	var snapshot map[string]json.RawMessage
	if err = json.Unmarshal(scene.SceneJSON, &snapshot); err != nil && len(scene.SceneJSON) > 0 {
		http.Error(w, "scene metadata is invalid", http.StatusConflict)
		return
	}
	if snapshot == nil {
		snapshot = make(map[string]json.RawMessage)
	}
	snapshot["generation_brief"], _ = json.Marshal(brief)
	scene.SceneJSON, _ = json.Marshal(snapshot)
	if err = s.database.UpdateScene(scene); err != nil {
		http.Error(w, "could not save scene look; reload and try again", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, scene)
}
