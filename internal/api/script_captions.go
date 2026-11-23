package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
)

// Captions from the script: the scene already knows what is said and in which shot,
// so the words never come from speech-to-text. Their times start as an estimate at shot
// level; a clip made with its own sound is then listened to (speech_timing.go) and each
// line moves to when it is really spoken.

type shotSpan struct {
	ID         string
	Start, End float64
}

type timedLine struct {
	ID, CharacterID, Language, Text string
	Start, End                      float64
	// Shot is the shot the line is assigned to; "" when it may fall anywhere in the clip.
	Shot string
	// Timing is timingHeard when the times come from the clip's own sound, otherwise
	// timingEstimated (spread over the line's shot by length).
	Timing string
	// Heard is what was said where the line was found, and Similarity how close it is
	// to the script (0-1). SpokenStart and SpokenEnd are the words themselves, without
	// a caption's reading margins.
	Heard                  string
	Similarity             float64
	SpokenStart, SpokenEnd float64
	// Scripted is the script's line when Text shows what was heard instead.
	Scripted string
}

// How a caption's time was found.
const (
	timingHeard     = "heard"     // when the line is spoken in the clip's own sound
	timingEstimated = "estimated" // spread over the line's shot by its length
	timingVoice     = "voice"     // when a voice-over line speaks it
)

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
			out = append(out, timedLine{ID: m.id, CharacterID: m.line.CharacterID, Language: m.line.Language, Text: strings.TrimSpace(m.line.Text), Start: round3(at), End: round3(at + show), Shot: key, Timing: timingEstimated})
			cursor += share
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

func round3(v float64) float64 { return float64(int64(v*1000+0.5)) / 1000 }

// clipScript is a rendered clip with the script lines it should speak, at their
// estimated times in the clip's own time.
type clipScript struct {
	Out    *models.RenderOutput
	Job    *models.RenderJob
	Scene  *models.Scene
	Lines  []timedLine
	Length float64
}

// outputScriptLines returns script captions in the rendered video's own time: heard
// times when the clip has its own sound and was listened to, estimated otherwise.
func (s *Server) outputScriptLines(ctx context.Context, project, outputID string) ([]timedLine, *clipScript, error) {
	plan, err := s.outputScript(project, outputID)
	if err != nil {
		return nil, nil, err
	}
	if len(plan.Lines) == 0 {
		return nil, plan, fmt.Errorf("“%s” has no script lines for this clip", plan.Scene.Title)
	}
	return s.heardLines(ctx, plan.Out, plan.Job, plan.Lines, plan.Length), plan, nil
}

// outputScript resolves a finished scene clip and places its script lines by estimate.
func (s *Server) outputScript(project, outputID string) (*clipScript, error) {
	out, err := s.database.GetRenderOutput(outputID)
	if err != nil {
		return nil, fmt.Errorf("video not found")
	}
	job, err := s.database.GetRenderJob(out.RenderJobID)
	if err != nil || job.ProjectID == nil || *job.ProjectID != project || job.Status != models.StatusSucceeded {
		return nil, fmt.Errorf("this video isn't a finished render in this project")
	}
	if job.SceneID == nil {
		return nil, fmt.Errorf("this video wasn't made from a scene, so there's no script to use")
	}
	scene, err := s.database.GetScene(*job.SceneID)
	if err != nil {
		return nil, fmt.Errorf("the scene for this video was deleted")
	}
	spoken := sceneSpokenLines(scene)
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
	lines := placeScriptLines(spoken, settings.DialogueShots, spans, length, !partial)
	return &clipScript{Out: out, Job: job, Scene: scene, Lines: lines, Length: length}, nil
}

// clipCue is a script line placed on the edit timeline, with when its words are
// spoken there (Spoken*, -1 when not heard) and whether the clip speaks it itself.
type clipCue struct {
	Cue                    composition.Cue
	SpokenStart, SpokenEnd float64
	// Native is set when the clip was made with its own sound, so its own voices say it.
	Native bool
	// ClipEnd is where the clip that says it ends on the timeline.
	ClipEnd float64
}

// scriptCuesForClip maps a rendered clip's script lines onto the edit timeline.
func (s *Server) scriptCuesForClip(ctx context.Context, project string, clip composition.Clip) ([]composition.Cue, error) {
	placed, err := s.clipCues(ctx, project, clip)
	if err != nil {
		return nil, err
	}
	cues := make([]composition.Cue, len(placed))
	for i, p := range placed {
		cues[i] = p.Cue
	}
	return cues, nil
}

// clipCues maps a rendered clip's script lines onto the edit timeline.
func (s *Server) clipCues(ctx context.Context, project string, clip composition.Clip) ([]clipCue, error) {
	if !strings.HasPrefix(clip.AssetID, "output-") {
		return nil, fmt.Errorf("not a rendered scene video")
	}
	lines, plan, err := s.outputScriptLines(ctx, project, strings.TrimPrefix(clip.AssetID, "output-"))
	if err != nil {
		return nil, err
	}
	speed := clip.Speed
	if speed <= 0 {
		speed = 1
	}
	from, to := clip.Start, clip.Start+clip.Duration
	at := func(t float64) float64 { return clip.Start + (t-clip.Trim)/speed }
	cues := []clipCue{}
	for _, l := range lines {
		start := max(from, at(l.Start))
		end := min(to, at(l.End))
		if end-start < 0.3 {
			continue // Trimmed away in this edit.
		}
		c := clipCue{Cue: composition.Cue{ID: "script-" + compositionHash([]byte(clip.ID + ":" + l.ID))[:24], UtteranceID: l.ID, CharacterID: l.CharacterID, Language: l.Language, Start: round3(start), End: round3(end), Text: l.Text, Timing: l.Timing, Scripted: l.Scripted}, SpokenStart: -1, SpokenEnd: -1, Native: nativeSound(plan.Job), ClipEnd: round3(to)}
		if c.Cue.Timing == "" {
			c.Cue.Timing = timingEstimated
		}
		if l.Timing == timingHeard {
			c.SpokenStart, c.SpokenEnd = round3(max(from, at(l.SpokenStart))), round3(min(to, at(l.SpokenEnd)))
		}
		cues = append(cues, c)
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
		cues, err := s.scriptCuesForClip(r.Context(), project, clip)
		if err != nil {
			skipped = append(skipped, map[string]string{"clip_id": clip.ID, "reason": err.Error()})
			continue
		}
		captions = append(captions, cues...)
	}
	followed := s.alignCaptionsToVoice(project, captions, body.Audio)
	sort.SliceStable(captions, func(i, j int) bool { return captions[i].Start < captions[j].Start })
	timing := map[string]int{timingHeard: 0, timingEstimated: 0, timingVoice: 0, "changed": 0}
	for _, c := range captions {
		timing[c.Timing]++
		if c.Scripted != "" {
			timing["changed"]++
		}
	}
	writeJSON(w, 200, map[string]any{"captions": captions, "skipped": skipped, "voice_timed": followed, "timing": timing})
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
		// A voice line speaks the script's words, so its caption shows them too.
		key := strings.TrimSpace(captions[i].Text)
		if captions[i].Scripted != "" {
			key = strings.TrimSpace(captions[i].Scripted)
		}
		if clips := spoken[key]; len(clips) > 0 {
			captions[i].Start, captions[i].End = round3(clips[0].Start), round3(clips[0].Start+clips[0].Duration)
			captions[i].Text, captions[i].Scripted, captions[i].Timing = key, "", timingVoice
			spoken[key] = clips[1:]
			followed++
		}
	}
	return followed
}
