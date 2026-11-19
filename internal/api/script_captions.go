package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
)

// Captions from the script: the scene already knows what is said and in which shot,
// so captions can be placed without paying for speech-to-text. Timing is an estimate
// at shot level (exact word timing still needs transcription or saved-voice alignment).

type shotSpan struct {
	ID         string
	Start, End float64
}

type timedLine struct {
	ID, CharacterID, Language, Text string
	Start, End                      float64
}

// placeScriptLines spreads each line across its assigned shot (or the whole clip when
// unassigned and includeUnassigned), splitting a shared window by text length.
func placeScriptLines(lines []SpokenLine, assign map[string]string, shots []shotSpan, length float64, includeUnassigned bool) []timedLine {
	if length <= 0 {
		return nil
	}
	span := map[string]shotSpan{}
	for _, s := range shots {
		span[s.ID] = s
	}
	type member struct {
		line SpokenLine
		id   string
	}
	windows := map[string][]member{}
	order := []string{}
	for i, line := range lines {
		if strings.TrimSpace(line.Text) == "" {
			continue
		}
		id := line.ID
		if id == "" {
			id = fmt.Sprintf("line-%d", i)
		}
		key, target := "", assign[id]
		if target != "" {
			if _, ok := span[target]; !ok {
				continue // Spoken in a different clip of this scene.
			}
			key = target
		} else if !includeUnassigned {
			continue
		}
		if _, seen := windows[key]; !seen {
			order = append(order, key)
		}
		windows[key] = append(windows[key], member{line, id})
	}
	out := []timedLine{}
	for _, key := range order {
		start, end := 0.0, length
		if key != "" {
			start, end = span[key].Start, span[key].End
		}
		if end-start > 0.6 { // Small breathing room so captions don't flash at cuts.
			start, end = start+0.1, end-0.1
		}
		total := 0
		for _, m := range windows[key] {
			total += max(1, len([]rune(m.line.Text)))
		}
		cursor := start
		for _, m := range windows[key] {
			runes := max(1, len([]rune(m.line.Text)))
			share := (end - start) * float64(runes) / float64(total)
			// Show a line about as long as it takes to read (~14 characters a second),
			// centered in its slot, instead of stretching a short line over a long shot.
			show := min(share, max(1.2, float64(runes)/14+0.6))
			at := cursor + (share-show)/2
			out = append(out, timedLine{ID: m.id, CharacterID: m.line.CharacterID, Language: m.line.Language, Text: strings.TrimSpace(m.line.Text), Start: round3(at), End: round3(at + show)})
			cursor += share
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

func round3(v float64) float64 { return float64(int64(v*1000+0.5)) / 1000 }

// outputScriptLines returns script captions in the rendered video's own time.
func (s *Server) outputScriptLines(project, outputID string) ([]timedLine, error) {
	out, err := s.database.GetRenderOutput(outputID)
	if err != nil {
		return nil, fmt.Errorf("video not found")
	}
	job, err := s.database.GetRenderJob(out.RenderJobID)
	if err != nil || job.ProjectID == nil || *job.ProjectID != project || job.Status != "succeeded" {
		return nil, fmt.Errorf("this video isn't a finished render in this project")
	}
	if job.SceneID == nil {
		return nil, fmt.Errorf("this video wasn't made from a scene, so there's no script to use")
	}
	scene, err := s.database.GetScene(*job.SceneID)
	if err != nil {
		return nil, fmt.Errorf("the scene for this video was deleted")
	}
	var script struct {
		Lines []SpokenLine `json:"spoken_lines"`
	}
	_ = json.Unmarshal(scene.SceneJSON, &script)
	// A silent render still has its script: silent footage plus a separate narrator
	// voice-over (and captions for it) is a normal way to make narrated videos.
	settings := sceneProduction(scene)
	shots, err := s.database.ListShots(scene.ID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(shots, func(i, j int) bool { return shots[i].ShotOrder < shots[j].ShotOrder })
	unit := renderRequestUnit(job.RequestJSON)
	ids := unit.ShotIDs
	if job.ShotID != nil {
		ids = []string{*job.ShotID}
	}
	chosen := []models.Shot{}
	if len(ids) > 0 {
		byID := map[string]models.Shot{}
		for _, sh := range shots {
			byID[sh.ID] = sh
		}
		for _, id := range ids {
			if sh, ok := byID[id]; ok {
				chosen = append(chosen, sh)
			}
		}
	} else {
		chosen = shots
	}
	sum := 0
	for _, sh := range chosen {
		sum += sh.Duration
	}
	length := float64(unit.Length)
	if length <= 0 {
		length = float64(sum)
	}
	if length <= 0 {
		length = float64(scene.Duration)
	}
	spans := []shotSpan{}
	if sum > 0 {
		scale, at := length/float64(sum), 0.0
		for _, sh := range chosen {
			d := float64(sh.Duration) * scale
			spans = append(spans, shotSpan{ID: sh.ID, Start: at, End: at + d})
			at += d
		}
	}
	// With several clips per scene, an unassigned line could belong to any of them.
	partial := unit.Count > 1 || job.ShotID != nil
	lines := placeScriptLines(script.Lines, settings.DialogueShots, spans, length, !partial)
	if len(lines) == 0 {
		return nil, fmt.Errorf("“%s” has no script lines for this clip", scene.Title)
	}
	return lines, nil
}

// scriptCuesForClip maps a rendered clip's script lines onto the edit timeline.
func (s *Server) scriptCuesForClip(project string, clip composition.Clip) ([]composition.Cue, error) {
	if !strings.HasPrefix(clip.AssetID, "output-") {
		return nil, fmt.Errorf("not a rendered scene video")
	}
	lines, err := s.outputScriptLines(project, strings.TrimPrefix(clip.AssetID, "output-"))
	if err != nil {
		return nil, err
	}
	speed := clip.Speed
	if speed <= 0 {
		speed = 1
	}
	from, to := clip.Start, clip.Start+clip.Duration
	cues := []composition.Cue{}
	for _, l := range lines {
		start := max(from, clip.Start+(l.Start-clip.Trim)/speed)
		end := min(to, clip.Start+(l.End-clip.Trim)/speed)
		if end-start < 0.3 {
			continue // Trimmed away in this edit.
		}
		cues = append(cues, composition.Cue{ID: "script-" + compositionHash([]byte(clip.ID + ":" + l.ID))[:24], UtteranceID: l.ID, CharacterID: l.CharacterID, Language: l.Language, Start: round3(start), End: round3(end), Text: l.Text})
	}
	return cues, nil
}

func (s *Server) handleCompositionScriptCaptions(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	var body struct {
		Clips []composition.Clip `json:"clips"`
		// Audio lets captions follow narrator voice lines placed by Voice from script.
		Audio []composition.Clip `json:"audio"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body) != nil {
		http.Error(w, "invalid clips", 400)
		return
	}
	captions := []composition.Cue{}
	skipped := []map[string]string{}
	for _, clip := range body.Clips {
		if !strings.HasPrefix(clip.AssetID, "output-") {
			continue // Images and uploaded media have no script.
		}
		cues, err := s.scriptCuesForClip(project, clip)
		if err != nil {
			skipped = append(skipped, map[string]string{"clip_id": clip.ID, "reason": err.Error()})
			continue
		}
		captions = append(captions, cues...)
	}
	followed := s.alignCaptionsToVoice(project, captions, body.Audio)
	sort.SliceStable(captions, func(i, j int) bool { return captions[i].Start < captions[j].Start })
	writeJSON(w, 200, map[string]any{"captions": captions, "skipped": skipped, "voice_timed": followed})
}

// alignCaptionsToVoice times each caption to the voice-over clip that speaks the same
// line, so captions stay with the narrator even after lines were spaced apart.
func (s *Server) alignCaptionsToVoice(project string, captions []composition.Cue, audio []composition.Clip) int {
	spoken := map[string][]composition.Clip{}
	for _, clip := range audio {
		if !strings.HasPrefix(clip.AssetID, "voiceover_") {
			continue
		}
		a, err := s.database.GetAsset(clip.AssetID)
		if err != nil || a.ProjectID == nil || *a.ProjectID != project {
			continue
		}
		var meta struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(a.MetadataJSON, &meta) == nil && strings.TrimSpace(meta.Text) != "" {
			key := strings.TrimSpace(meta.Text)
			spoken[key] = append(spoken[key], clip)
		}
	}
	followed := 0
	for i := range captions {
		key := strings.TrimSpace(captions[i].Text)
		if clips := spoken[key]; len(clips) > 0 {
			captions[i].Start, captions[i].End = round3(clips[0].Start), round3(clips[0].Start+clips[0].Duration)
			spoken[key] = clips[1:]
			followed++
		}
	}
	return followed
}

func (s *Server) handleOutputScriptCaptions(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	lines, err := s.outputScriptLines(project, chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	segments := []models.CaptionSegment{}
	for _, l := range lines {
		segments = append(segments, models.CaptionSegment{ID: l.ID, CharacterID: l.CharacterID, Language: l.Language, Start: l.Start, End: l.End, Text: l.Text})
	}
	writeJSON(w, 200, map[string]any{"segments": segments})
}
