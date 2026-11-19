package api

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
)

func TestVoiceoverSpeaksScriptLinesAtCaptionTimingAndCaches(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	sample := filepath.Join(t.TempDir(), "tone.mp3")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.8", "-y", sample).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	mp3, _ := os.ReadFile(sample)
	calls := 0
	old := voiceoverSpeech
	voiceoverSpeech = func(_ context.Context, provider, _, _, model, voice, text string) ([]byte, error) {
		calls++
		if provider != "fish" || model != "s1" || voice != "narrator1" || text == "" {
			t.Fatalf("unexpected speech call %s %s %s %q", provider, model, voice, text)
		}
		return mp3, nil
	}
	t.Cleanup(func() { voiceoverSpeech = old })

	s := routerServer(t, nil)
	s.cfg.FishAPIKey = "test"
	project, _ := s.database.GetActiveProjectID()
	sceneJSON, _ := json.Marshal(map[string]any{"spoken_lines": []map[string]any{
		{"id": "l1", "text": "Where did the towers come from?", "language": "English"},
		{"id": "l2", "text": "Let's go back in time!", "language": "English"},
	}, "production": map[string]any{"dialogue_shots": map[string]string{"l1": "shot-1", "l2": "shot-2"}}})
	scene := &models.Scene{ProjectID: &project, Title: "Towers", Duration: 6, AspectRatio: "9:16", SceneJSON: sceneJSON}
	if e := s.database.CreateScene(scene); e != nil {
		t.Fatal(e)
	}
	for i, d := range []int{2, 4} {
		if e := s.database.CreateShot(&models.Shot{ID: []string{"shot-1", "shot-2"}[i], SceneID: scene.ID, ShotOrder: i + 1, Duration: d, Prompt: "p"}); e != nil {
			t.Fatal(e)
		}
	}
	request, _ := json.Marshal(map[string]any{"render_plan_id": "plan", "render_unit": map[string]any{"key": "plan:0", "index": 0, "count": 1, "shot_ids": []string{"shot-1", "shot-2"}, "length": 6, "duration": 6}})
	job := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Status: "succeeded", RequestJSON: request}
	if e := s.database.CreateRenderJob(job); e != nil {
		t.Fatal(e)
	}
	out := &models.RenderOutput{RenderJobID: job.ID, VideoPath: "outputs/x.mp4"}
	if e := s.database.CreateRenderOutput(out); e != nil {
		t.Fatal(e)
	}
	clip := composition.Clip{ID: "clip-1", AssetID: "output-" + out.ID, Start: 10, Trim: 1, Duration: 5, Speed: 1}

	// Without any voice the lines are reported, not silently dropped, and nothing is paid.
	body, _ := json.Marshal(map[string]any{"clips": []any{clip}})
	w := call(t, s, "POST", "/compositions/voiceover", string(body))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "No voice for Narrator") || calls != 0 {
		t.Fatal(w.Code, w.Body.String(), calls)
	}

	body, _ = json.Marshal(map[string]any{"clips": []any{clip}, "narrator": map[string]string{"provider": "fish", "voice_id": "narrator1", "model": "s1"}})
	var result struct {
		Items     []voiceoverItem `json:"items"`
		Generated int             `json:"generated"`
		Reused    int             `json:"reused"`
	}
	w = call(t, s, "POST", "/compositions/voiceover", string(body))
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Items) != 2 || result.Generated != 2 || calls != 2 {
		t.Fatal(w.Code, w.Body.String())
	}
	if result.Items[0].Start != 10 || result.Items[1].Start < 11 || result.Items[0].Duration <= 0 {
		t.Fatalf("voice not placed at caption timing: %+v", result.Items)
	}
	// The voice files are ordinary editor audio sources.
	_, sources, err := s.compositionSources(project)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, src := range sources {
		found = found || (src.ID == result.Items[0].AssetID && src.MediaKind == "audio")
	}
	if !found {
		t.Fatal("voice-over asset is not an editor audio source")
	}
	// Same lines, same voice: reused, not paid again.
	w = call(t, s, "POST", "/compositions/voiceover", string(body))
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Reused != 2 || result.Generated != 0 || calls != 2 {
		t.Fatal(w.Code, w.Body.String(), calls)
	}
}
