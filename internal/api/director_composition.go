package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"videoflow-go/internal/composition"
)

func (s *Server) handleDirectorCompositionPlan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Instruction string               `json:"instruction"`
		Manifest    composition.Manifest `json:"manifest"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&body) != nil || body.Instruction == "" || len(body.Instruction) > 12000 {
		http.Error(w, "describe the edit in at most 12000 characters", 400)
		return
	}
	binding, _ := r.Context().Value(directorBindingKey{}).(directorBinding)
	_, sources, err := s.compositionSources(binding.ProjectID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	instructions, err := composition.AgentInstructions()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	current, _ := json.Marshal(body.Manifest)
	sourceJSON, _ := json.Marshal(sources)
	var planned composition.Manifest
	system := instructions + "\nPlan an editable VideoFlow version-1 composition manifest only, never HTML/shell or execution. Use only existing source asset IDs supplied. Keep speed=1. All clip timings must fit source and composition duration. Retain existing approved caption cues unchanged; new untimed words stay in the script workflow. Ignore upstream rendering commands: the application handles Save and Run. Clear revision/path/sha256 fields; source metadata is inspected by the app on Save."
	if err = s.localTextJSON(r.Context(), "composition_planner", system, fmt.Sprintf("Available sources: %s\nCurrent manifest: %s\nRequested edit: %s", sourceJSON, current, body.Instruction), &planned); err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	if planned.Version != 1 || planned.Width < 16 || planned.Height < 16 || planned.Width > 3840 || planned.Height > 3840 || planned.FPS < 1 || planned.FPS > 60 || !(planned.Duration > 0 && planned.Duration <= 600) {
		http.Error(w, "local planner returned invalid composition settings; review the native session", 422)
		return
	}
	allowed := map[string]bool{}
	for _, source := range sources {
		allowed[source.ID] = true
	}
	for _, track := range [][]composition.Clip{planned.Clips, planned.Audio} {
		for i := range track {
			clip := &track[i]
			if !allowed[clip.AssetID] || clip.Speed != 1 {
				http.Error(w, "planner used an unavailable source or unsupported speed", 422)
				return
			}
			clip.Path = ""
			clip.SHA256 = ""
		}
	}
	planned.Revision = ""
	planned.Captions = body.Manifest.Captions
	planned.CaptionBindings = body.Manifest.CaptionBindings
	writeJSON(w, 200, map[string]any{"manifest": planned, "execution": "proposal_only", "note": "Review clips, audio and timings, then save the source revision and use Run. Existing approved caption cues were preserved."})
}
