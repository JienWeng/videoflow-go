package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
)

// A joined film gets one music bed from its story's sound plan (sound_plan.go), lowered
// while someone speaks, and short fades at the cuts between the clips' own sound
// (composition.Score). The bed plays the plan's track: an upload, a library track, or
// one made once with earshot from the plan's direction when earshot is installed.

// makeMusicBed makes a music track from words with earshot. Tests replace it.
var makeMusicBed = composition.MakeSound

// bedLength is how long a music file is. Tests replace it.
var bedLength = func(ctx context.Context, path string) (float64, error) {
	seconds, _, _, _, err := composition.Probe(ctx, path)
	return seconds, err
}

// bedSeconds is how long a music track made from words is; it loops under longer films.
const bedSeconds = 30

// storyScore is what the soundtrack of a joined story is, for the edit stage and chat.
type storyScore struct {
	composition.Soundtrack
	// Speech is how the ducking times were found: "heard" (every speaking clip was
	// listened to), "estimated", or "mixed".
	Speech string `json:"speech_timing,omitempty"`
	// Missing says what to do when the plan wants music the film does not have yet.
	Missing string `json:"missing,omitempty"`
}

// filmSpeech is when someone speaks in a joined film: each clip that speaks with its
// own sound at the times it was heard (remembered on the clip; nothing is listened to
// here), else as estimated from the script, and each voice-over track.
func (s *Server) filmSpeech(project string, m composition.Manifest) ([]composition.Span, string) {
	spans := []composition.Span{}
	heard, estimated := 0, 0
	for _, clip := range m.Clips {
		if !strings.HasPrefix(clip.AssetID, "output-") || clip.Muted || !clip.HasAudio {
			continue
		}
		plan, err := s.outputScript(project, strings.TrimPrefix(clip.AssetID, "output-"))
		if err != nil || !nativeSound(plan.Job) {
			continue
		}
		lines := plan.Lines
		if saved := savedHearing(plan.Out.QAJSON, plan.Out.VideoPath); saved != nil {
			lines = placeHeardLines(lines, saved.Words, plan.Length)
		}
		from, to := clip.Start, clip.Start+clip.Duration
		at := func(t float64) float64 { return clip.Start + t - clip.Trim }
		for _, l := range lines {
			start, end := l.Start, l.End
			if l.Timing == timingHeard && l.SpokenEnd > l.SpokenStart {
				start, end = l.SpokenStart, l.SpokenEnd
				heard++
			} else {
				estimated++
			}
			if a, b := max(from, at(start)), min(to, at(end)); b > a {
				spans = append(spans, composition.Span{Start: a, End: b})
			}
		}
	}
	for _, a := range m.Audio {
		if strings.HasPrefix(a.AssetID, "voiceover_") && !a.Muted {
			spans = append(spans, composition.Span{Start: a.Start, End: a.Start + a.Duration})
			heard++
		}
	}
	timing := ""
	switch {
	case heard > 0 && estimated > 0:
		timing = "mixed"
	case heard > 0:
		timing = timingHeard
	case estimated > 0:
		timing = timingEstimated
	}
	return spans, timing
}

// storyBed is the music track a story's bed plays, nil when there is none yet.
func (s *Server) storyBed(ctx context.Context, project string, plan soundPlan) *composition.Bed {
	if plan.Music.Mode != musicBed || plan.Music.Source == "" {
		return nil
	}
	a, err := s.database.GetAsset(plan.Music.Source)
	if err != nil || a.ProjectID == nil || *a.ProjectID != project {
		return nil
	}
	path, err := s.storagePath(a.FilePath)
	if err != nil {
		return nil
	}
	seconds, err := bedLength(ctx, path)
	if err != nil || seconds <= 0 {
		slog.Warn("the music bed's length could not be read", "asset", a.ID, "reason", errText(err))
		return nil
	}
	return &composition.Bed{AssetID: a.ID, Seconds: seconds}
}

func errText(err error) string {
	if err == nil {
		return "no length"
	}
	return err.Error()
}

// scoreStory lays the story's music bed under its joined film and fades the cuts.
func (s *Server) scoreStory(ctx context.Context, project, scriptID string, m composition.Manifest) (composition.Manifest, storyScore) {
	plan, _ := s.storySound(ctx, project, scriptID)
	speech, timing := s.filmSpeech(project, m)
	bed := s.storyBed(ctx, project, plan)
	out, track, err := composition.Score(m, bed, speech)
	if err != nil {
		slog.Warn("the music bed was left out", "script", scriptID, "reason", err.Error())
		out, track, _ = composition.Score(m, nil, speech)
	}
	score := storyScore{Soundtrack: track, Speech: timing}
	if plan.Music.Mode == musicBed && !track.Music {
		score.Missing = "add music: upload a track in Edit video under Audio, or let it be made once when the scenes are joined (needs earshot)"
	}
	return out, score
}

// ensureMusicBed makes the story's music bed once when its plan wants one and has no
// track yet: with earshot (free, on the CPU), kept as an asset and in the plan. It
// returns false when there is still no track.
func (s *Server) ensureMusicBed(ctx context.Context, project, scriptID string) bool {
	plan, _ := s.storySound(ctx, project, scriptID)
	if plan.Music.Mode != musicBed {
		return false
	}
	if plan.Music.Source != "" {
		return true
	}
	sum := sha256.Sum256([]byte(scriptID + "\x00" + plan.Music.Prompt))
	id := "musicbed_" + hex.EncodeToString(sum[:])[:16]
	if a, err := s.database.GetAsset(id); err == nil && a.ProjectID != nil && *a.ProjectID == project {
		plan.Music.Source = id // made before for this direction
		_, err = s.saveSoundPlan(ctx, project, scriptID, plan)
		return err == nil
	}
	target := filepath.Join(s.cfg.StorageRoot, "assets", id+".wav")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return false
	}
	words := strings.TrimRight(plan.Music.Prompt, ". ") + ", instrumental music, no vocals, a seamless loop"
	if err := makeMusicBed(ctx, words, bedSeconds, target); err != nil {
		slog.Info("no music bed was made; the story needs a track", "script", scriptID, "reason", err.Error())
		return false
	}
	meta, _ := json.Marshal(map[string]any{"made_with": "earshot", "earshot_commit": composition.EarshotCommit, "words": words, "seconds": bedSeconds, "script_id": scriptID})
	a := &models.Asset{ID: id, ProjectID: &project, Type: musicBedAsset, Name: "Music bed · " + truncate(plan.Music.Prompt, 80), FilePath: target, TagsJSON: []byte(`["music"]`), MetadataJSON: meta}
	if err := s.database.CreateUploadedAsset(a); err != nil {
		_ = os.Remove(target)
		return false
	}
	plan.Music.Source = id
	if _, err := s.saveSoundPlan(ctx, project, scriptID, plan); err != nil {
		slog.Warn("the music bed was made but not kept in the sound plan", "script", scriptID, "reason", err.Error())
		return false
	}
	return true
}
