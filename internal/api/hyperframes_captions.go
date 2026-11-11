package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
)

func compositionHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
func (s *Server) importCompositionCaptions(project string, clip composition.Clip) ([]composition.Cue, composition.CaptionBinding, error) {
	var binding composition.CaptionBinding
	if !composition.ValidID(clip.ID) || !strings.HasPrefix(clip.AssetID, "output-") || clip.Speed != 1 || clip.Trim < 0 || clip.Start < 0 || clip.Duration <= 0 {
		return nil, binding, fmt.Errorf("choose a normal-speed rendered clip before reusing its approved captions")
	}
	id := strings.TrimPrefix(clip.AssetID, "output-")
	out, e := s.database.GetRenderOutput(id)
	if e != nil {
		return nil, binding, fmt.Errorf("caption output unavailable")
	}
	job, e := s.database.GetRenderJob(out.RenderJobID)
	if e != nil || job.ProjectID == nil || *job.ProjectID != project || job.Status != "succeeded" {
		return nil, binding, fmt.Errorf("caption output is not completed in this project")
	}
	var payload struct {
		Source   string                  `json:"source"`
		Reviewed bool                    `json:"reviewed"`
		Bindings json.RawMessage         `json:"source_bindings"`
		Segments []models.CaptionSegment `json:"segments"`
	}
	if json.Unmarshal(out.CaptionsJSON, &payload) != nil || payload.Source != "approved_script" || !payload.Reviewed || len(payload.Bindings) == 0 || string(payload.Bindings) == "[]" || len(payload.Segments) == 0 {
		return nil, binding, fmt.Errorf("this output needs reviewed approved-script alignment before captions can be reused")
	}
	if e = s.ValidateScriptCaptionSource(out); e != nil {
		return nil, binding, e
	}
	video, _, e := s.voiceOutputBinding(project, id)
	if e != nil {
		return nil, binding, e
	}
	binding = composition.CaptionBinding{ClipID: clip.ID, OutputID: id, CaptionSHA256: compositionHash(out.CaptionsJSON), VideoSHA256: video.Hash, SourceBindings: payload.Bindings, Start: clip.Start, Trim: clip.Trim, Duration: clip.Duration}
	cues := []composition.Cue{}
	for _, cue := range payload.Segments {
		if cue.End <= clip.Trim || cue.Start >= clip.Trim+clip.Duration {
			continue
		}
		if cue.Start < clip.Trim || cue.End > clip.Trim+clip.Duration {
			return nil, binding, fmt.Errorf("trim cuts through approved caption %s; adjust the trim or review the source captions", cue.ID)
		}
		if cue.ID == "" {
			return nil, binding, fmt.Errorf("approved captions need stable spoken-line IDs")
		}
		cues = append(cues, composition.Cue{ID: "cue-" + compositionHash([]byte(clip.ID + ":" + cue.ID))[:32], SourceClipID: clip.ID, UtteranceID: cue.ID, CharacterID: cue.CharacterID, Language: cue.Language, Start: clip.Start + cue.Start - clip.Trim, End: clip.Start + cue.End - clip.Trim, Text: cue.Text})
	}
	if len(cues) == 0 {
		return nil, binding, fmt.Errorf("no approved caption cues fall within this clip")
	}
	return cues, binding, nil
}
func (s *Server) validateCompositionCaptions(project string, m composition.Manifest) error {
	seen := map[string]bool{}
	for _, binding := range m.CaptionBindings {
		if seen[binding.ClipID] {
			return fmt.Errorf("duplicate caption source binding")
		}
		seen[binding.ClipID] = true
		var clip *composition.Clip
		for i := range m.Clips {
			if m.Clips[i].ID == binding.ClipID {
				clip = &m.Clips[i]
				break
			}
		}
		if clip == nil {
			return fmt.Errorf("caption source layer removed; remove its linked captions before saving")
		}
		expected, current, e := s.importCompositionCaptions(project, *clip)
		if e != nil {
			return e
		}
		currentJSON, _ := json.Marshal(current)
		bindingJSON, _ := json.Marshal(binding)
		if string(currentJSON) != string(bindingJSON) {
			return fmt.Errorf("caption source or clip timing changed; reuse approved captions again before saving/rendering")
		}
		actual := []composition.Cue{}
		for _, cue := range m.Captions {
			if cue.SourceClipID == binding.ClipID {
				actual = append(actual, cue)
			}
		}
		if !reflect.DeepEqual(expected, actual) {
			return fmt.Errorf("linked captions changed; edit approved captions at their source and import them again")
		}
	}
	for _, cue := range m.Captions {
		if cue.SourceClipID != "" && !seen[cue.SourceClipID] {
			return fmt.Errorf("caption source binding missing")
		}
	}
	return nil
}
func (s *Server) handleCompositionCaptionImport(w http.ResponseWriter, r *http.Request) {
	project, e := s.activeProjectID(r.Context())
	if e != nil {
		http.Error(w, "no active project", 400)
		return
	}
	var body struct {
		Clip composition.Clip `json:"clip"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body) != nil {
		http.Error(w, "invalid clip", 400)
		return
	}
	cues, binding, e := s.importCompositionCaptions(project, body.Clip)
	if e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	writeJSON(w, 200, map[string]any{"captions": cues, "binding": binding})
}
