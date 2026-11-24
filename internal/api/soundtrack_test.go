package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
)

// spokenClip is a finished native-sound clip of a scene of the story, saying text in
// its one 4-second shot; heard is when it was heard saying it (nil: never listened to).
func spokenClip(t *testing.T, s *Server, project, scriptID string, order int, text string, heard []float64) composition.Clip {
	t.Helper()
	shot := fmt.Sprintf("shot-%d", order)
	sceneJSON, _ := json.Marshal(map[string]any{"spoken_lines": []map[string]any{{"id": "l", "text": text, "language": "English"}}, "production": map[string]any{"audio_mode": "native", "dialogue_shots": map[string]string{"l": shot}}})
	scene := &models.Scene{ProjectID: &project, ScriptID: &scriptID, SceneOrder: order, Title: shot, Duration: 4, AspectRatio: "9:16", SceneJSON: sceneJSON}
	if e := s.database.CreateScene(scene); e != nil {
		t.Fatal(e)
	}
	if e := s.database.CreateShot(&models.Shot{ID: shot, SceneID: scene.ID, ShotOrder: 1, Duration: 4, Prompt: "p"}); e != nil {
		t.Fatal(e)
	}
	request, _ := json.Marshal(map[string]any{"generate_audio": true, "audio_mode": "native", "voice_policy": map[string]any{"mode": "native"}, "render_unit": map[string]any{"key": "plan:0", "index": 0, "count": 1, "shot_ids": []string{shot}, "length": 4, "duration": 4}})
	job := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Status: "succeeded", RequestJSON: request}
	if e := s.database.CreateRenderJob(job); e != nil {
		t.Fatal(e)
	}
	video := fmt.Sprintf("outputs/%d.mp4", order)
	out := &models.RenderOutput{RenderJobID: job.ID, VideoPath: video}
	if heard != nil {
		words := []map[string]any{}
		for i, w := range strings.Fields(text) {
			words = append(words, map[string]any{"word": w, "start": heard[0] + float64(i)*0.3, "end": heard[0] + float64(i)*0.3 + 0.25})
		}
		out.QAJSON, _ = json.Marshal(map[string]any{"speech_timing": map[string]any{"video": video, "words": words}})
	}
	if e := s.database.CreateRenderOutput(out); e != nil {
		t.Fatal(e)
	}
	return composition.Clip{ID: fmt.Sprintf("clip-%d", order), AssetID: "output-" + out.ID, Start: float64((order - 1) * 4), Duration: 4, OriginalDuration: 4, Speed: 1, HasAudio: true}
}

func TestStoryFilmGetsOneDuckedMusicBedThatTheEditorAccepts(t *testing.T) {
	s := routerServer(t, nil)
	project, _ := s.database.GetActiveProjectID()
	script := &models.Script{ProjectID: &project, Idea: "x", Title: "Towers", Summary: "x", DraftJSON: []byte("{}")}
	if e := s.database.CreateScript(script); e != nil {
		t.Fatal(e)
	}
	// Clip 1 was listened to (heard at 1.0 s), clip 2 was not (estimated from its shot).
	m := composition.Manifest{Version: 1, Width: 720, Height: 1280, FPS: 24, Duration: 12, CaptionStyle: "default", Audio: []composition.Clip{}, Captions: []composition.Cue{}}
	m.Clips = append(m.Clips, spokenClip(t, s, project, script.ID, 1, "Where did the towers come from?", []float64{1.0}))
	m.Clips = append(m.Clips, spokenClip(t, s, project, script.ID, 2, "Back in time we go.", nil))
	m.Clips = append(m.Clips, spokenClip(t, s, project, script.ID, 3, "", nil))

	previous := bedLength
	bedLength = func(context.Context, string) (float64, error) { return 20, nil }
	t.Cleanup(func() { bedLength = previous })

	// No track yet: the edit has no bed and says how to add one.
	if _, err := s.saveSoundPlan(t.Context(), project, script.ID, soundPlan{Music: soundMusic{Mode: musicBed, Prompt: "slow sparse piano"}, ClipAudio: clipDialogueAmbience, DecidedBy: decidedOwner}); err != nil {
		t.Fatal(err)
	}
	if ok := s.ensureMusicBed(t.Context(), project, script.ID); ok {
		t.Fatal("a bed was made without earshot")
	}
	_, score := s.scoreStory(t.Context(), project, script.ID, m)
	if score.Music || !strings.HasPrefix(score.Missing, "add music") || score.CutFades != 2 {
		t.Fatalf("score without a track: %+v", score)
	}

	// earshot (faked) makes the bed once; it is kept as an asset and in the plan.
	made := 0
	keep := makeMusicBed
	makeMusicBed = func(_ context.Context, words string, seconds int, out string) error {
		made++
		if !strings.Contains(words, "slow sparse piano") || seconds != bedSeconds || filepath.Ext(out) != ".wav" {
			t.Errorf("earshot asked %q %d %s", words, seconds, out)
		}
		return os.WriteFile(out, []byte("RIFF"), 0600)
	}
	t.Cleanup(func() { makeMusicBed = keep })
	if !s.ensureMusicBed(t.Context(), project, script.ID) || !s.ensureMusicBed(t.Context(), project, script.ID) || made != 1 {
		t.Fatalf("the bed was made %d times", made)
	}
	plan, _ := s.storySound(t.Context(), project, script.ID)
	if a, err := s.database.GetAsset(plan.Music.Source); err != nil || a.Type != musicBedAsset {
		t.Fatalf("the bed is not kept: %+v %v", plan, err)
	}
	if line := s.musicLine(project, plan); !strings.Contains(line, "one bed under the whole film from a track made once with earshot") {
		t.Errorf("music line %q", line)
	}

	scored, score := s.scoreStory(t.Context(), project, script.ID, m)
	if !score.Music || !score.Continuous || score.Speech != "mixed" || score.CutFades != 2 || score.Missing != "" || score.AssetID != plan.Music.Source {
		t.Fatalf("score %+v", score)
	}
	level := func(at float64) float64 {
		gain := 0.0
		for _, a := range scored.Audio {
			if strings.HasPrefix(a.ID, composition.BedPrefix) && at >= a.Start && at < a.Start+a.Duration {
				g := *a.Volume
				if in := at - a.Start; a.FadeIn > 0 && in < a.FadeIn {
					g *= in / a.FadeIn
				}
				if out := a.Start + a.Duration - at; a.FadeOut > 0 && out < a.FadeOut {
					g *= out / a.FadeOut
				}
				gain += g
			}
		}
		return gain
	}
	// Heard: "Where did the towers come from?" is said from 1.0 to 2.75 s.
	for _, at := range []float64{1.0, 1.9, 2.7} {
		if g := level(at); math.Abs(g-composition.DuckLevel) > 0.002 {
			t.Errorf("music at %.2f while the heard line is said at %.1fs", g, at)
		}
	}
	// The silent third clip has the music back up.
	if g := level(10); math.Abs(g-composition.BedLevel) > 0.002 {
		t.Errorf("music at %.2f in the clip without speech", g)
	}

	// The editor's own validation accepts the scored film.
	sources, _, err := s.compositionSources(project)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, c := range scored.Clips {
		sources[c.AssetID] = filepath.Join(dir, c.ID+".mp4")
		os.WriteFile(sources[c.AssetID], []byte(c.ID), 0600)
	}
	if _, ok := sources[plan.Music.Source]; !ok {
		t.Fatalf("the bed is not an edit source: %v", sources)
	}
	if _, err := composition.Save(t.TempDir(), project, scored, sources); err != nil {
		t.Fatalf("the editor refuses the scored film: %v", err)
	}
}
