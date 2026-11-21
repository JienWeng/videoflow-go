package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"videoflow-go/internal/composition"
)

func (s *Server) handleDirectorCompositionPlan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Instruction  string               `json:"instruction"`
		Manifest     composition.Manifest `json:"manifest"`
		SelectedClip string               `json:"selected_clip_id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&body) != nil || body.Instruction == "" || len(body.Instruction) > 12000 {
		http.Error(w, "describe the edit in at most 12000 characters", 400)
		return
	}
	binding, _ := r.Context().Value(directorBindingKey{}).(directorBinding)
	paths, sources, err := s.compositionSources(binding.ProjectID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	instructions, err := composition.AgentInstructions()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	instructions += "\nVideoFlow clip editing fields: muted (boolean), volume (0–1, default 1), fade_in and fade_out in seconds (nonnegative, sum must fit duration); use about 0.15 volume for background music below dialogue; preserve existing audio mix values unless asked to change them; optional adjustments object with all fields fit (contain or cover), scale (1–4), x and y focal position (0–100), brightness/contrast/saturation (0–2), opacity (0–1). Neutral defaults are fit=contain, scale=1, x=50, y=50, brightness=1, contrast=1, saturation=1, opacity=1. Preserve existing adjustments unless asked to change them. Split clips by adjusting start, trim and duration without modifying source assets. Do not change linked caption timing or its clip IDs; ask the user to remove and reimport linked captions first."
	current, _ := json.Marshal(body.Manifest)
	sourceJSON, _ := json.Marshal(sources)
	var planned composition.Manifest
	system := instructions + "\nPlan an editable VideoFlow version-1 composition manifest only, never HTML/shell or execution. Use only existing source asset IDs supplied. Keep speed=1. All clip timings must fit source and composition duration. Retain linked approved caption cues unchanged. You may correct text on existing unlinked captions, keeping original IDs and exact timings. Never invent timed cues. New untimed words stay in the script workflow. Ignore upstream rendering commands: the application handles Save and Run. Clear revision/path/sha256 fields; source metadata is inspected by the app on Save."
	ctx := context.WithValue(r.Context(), directorUserMessageKey{}, body.Instruction)
	if err = s.localTextJSON(ctx, "composition_planner", system, fmt.Sprintf("Available sources: %s\nCurrent manifest: %s\nSelected clip: %s\nRequested edit: %s", sourceJSON, current, body.SelectedClip, body.Instruction), &planned); err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	if planned.Version != 1 || planned.Width < 16 || planned.Height < 16 || planned.Width > 3840 || planned.Height > 3840 || planned.FPS < 1 || planned.FPS > 60 || !(planned.Duration > 0 && planned.Duration <= 600) {
		http.Error(w, "the AI suggested an edit the editor can't apply. Try asking again in different words", 422)
		return
	}
	allowed := map[string]bool{}
	seen := map[string]bool{}
	for _, source := range sources {
		allowed[source.ID] = true
	}
	for _, track := range [][]composition.Clip{planned.Clips, planned.Audio} {
		for i := range track {
			clip := &track[i]
			if !allowed[clip.AssetID] || clip.Speed != 1 || !composition.ValidID(clip.ID) || seen[clip.ID] || !(clip.Start >= 0 && clip.Trim >= 0 && clip.Duration > 0 && clip.Start+clip.Duration <= planned.Duration) || (clip.Adjustments != nil && !clip.Adjustments.Valid()) || (clip.Volume != nil && !(*clip.Volume >= 0 && *clip.Volume <= 1)) || !(clip.FadeIn >= 0 && clip.FadeOut >= 0 && clip.FadeIn+clip.FadeOut <= clip.Duration) {
				http.Error(w, "planner used an unavailable source or unsupported speed", 422)
				return
			}
			seen[clip.ID] = true
			if clip.Overlay != nil && !clip.Overlay.Valid() {
				http.Error(w, "invalid proposed overlay placement", 422)
				return
			}
			duration, width, height, audio, e := composition.Probe(r.Context(), paths[clip.AssetID])
			if e != nil || (duration > 0 && clip.Trim+clip.Duration > duration+.02) {
				http.Error(w, "proposed edit exceeds its source media", 422)
				return
			}
			clip.OriginalDuration = duration
			clip.Width = width
			clip.Height = height
			clip.HasAudio = audio
			clip.Path = ""
			clip.SHA256 = ""
		}
	}
	planned.Revision = ""
	proposedCaptions := planned.Captions
	planned.Captions = append([]composition.Cue{}, body.Manifest.Captions...)
	for i, original := range planned.Captions {
		for _, proposed := range proposedCaptions {
			if original.ID == proposed.ID && original.SourceClipID == "" && len(proposed.Text) > 0 && len(proposed.Text) <= 10000 {
				planned.Captions[i].Text = proposed.Text
			}
		}
	}
	for _, cue := range planned.Captions {
		if !(cue.Start >= 0 && cue.End > cue.Start && cue.End <= planned.Duration) {
			http.Error(w, "proposed timing cuts through existing captions", 422)
			return
		}
	}
	planned.CaptionVariants = body.Manifest.CaptionVariants
	if planned.Texts == nil {
		planned.Texts = body.Manifest.Texts
	}
	for _, layer := range planned.Texts {
		if !layer.Valid(planned.Duration) {
			http.Error(w, "planner returned an invalid title", 422)
			return
		}
	}
	writeJSON(w, 200, map[string]any{"manifest": planned, "execution": "proposal_only", "note": "Review clips, audio and timings, then save the source revision and use Run. Existing caption timings were kept."})
}
