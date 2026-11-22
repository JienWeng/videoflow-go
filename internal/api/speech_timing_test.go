package api

import (
	"context"
	"encoding/json"
	"testing"

	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

func spoken(pairs ...any) []providers.SpokenWord {
	out := []providers.SpokenWord{}
	for i := 0; i < len(pairs); i += 3 {
		out = append(out, providers.SpokenWord{Word: pairs[i].(string), Start: pairs[i+1].(float64), End: pairs[i+2].(float64)})
	}
	return out
}

// Each script line is found where it is heard, whatever the punctuation, a misheard
// word, or how the listener splits Chinese; a line nobody said is not placed.
func TestScriptLinesAreFoundWhereTheyAreHeard(t *testing.T) {
	words := spoken("It", 3.2, 3.4, "won't", 3.4, 3.7, "fly", 3.7, 4.0, "I", 4.3, 4.4, "give", 4.4, 4.6, "up", 4.6, 4.9,
		"Wait", 7.0, 7.4, "I", 7.6, 7.7, "now", 7.7, 7.9, "a", 7.9, 8.0, "secret", 8.0, 8.6)
	got := heardSpans([]string{"It won't fly. I give up.", "Wait. I know a secret.", "This line was never spoken at all."}, words)
	if got[0] != [2]float64{3.2, 4.9} || got[1] != [2]float64{7.0, 8.6} || got[2][0] >= 0 {
		t.Fatalf("english: %v", got)
	}
	chinese := spoken("他从", 1.0, 1.4, "一万年", 1.4, 2.0, "后", 2.0, 2.2, "回来", 2.2, 2.8, "守住", 6.0, 6.5, "机器", 6.5, 7.0)
	got = heardSpans([]string{"他从一万年后回来，", "守住机器——"}, chinese)
	if got[0] != [2]float64{1.0, 2.8} || got[1] != [2]float64{6.0, 7.0} {
		t.Fatalf("chinese: %v", got)
	}
	if got = heardSpans([]string{"Hello there"}, nil); got[0][0] >= 0 {
		t.Fatalf("silence: %v", got)
	}
}

// A clip made with its own sound is listened to once, its captions move to when the
// lines are really spoken, and the answer is remembered so it is not paid for twice.
func TestCaptionsFollowTheSpeechHeardInAClip(t *testing.T) {
	s, _ := falServer(t, "done")
	project, _ := s.database.GetActiveProjectID()
	sceneJSON, _ := json.Marshal(map[string]any{"spoken_lines": []map[string]any{
		{"id": "l1", "text": "It won't fly. I give up.", "language": "en"},
		{"id": "l2", "text": "Wait. I know a secret.", "language": "en"},
	}})
	scene := &models.Scene{ProjectID: &project, Title: "Kite", Duration: 10, AspectRatio: "9:16", SceneJSON: sceneJSON}
	if e := s.database.CreateScene(scene); e != nil {
		t.Fatal(e)
	}
	make := func(sound bool) *models.RenderOutput {
		request, _ := json.Marshal(map[string]any{"generate_audio": sound, "render_unit": map[string]any{"key": "plan:0", "count": 1, "length": 10, "duration": 10}})
		job := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Status: "succeeded", RequestJSON: request}
		if e := s.database.CreateRenderJob(job); e != nil {
			t.Fatal(e)
		}
		out := &models.RenderOutput{RenderJobID: job.ID, VideoPath: "storage/outputs/" + job.ID + ".mp4"}
		if e := s.database.CreateRenderOutput(out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	listened := 0
	old := listenToClip
	t.Cleanup(func() { listenToClip = old })
	listenToClip = func(_ *Server, _ context.Context, _, language, hint string) ([]providers.SpokenWord, float64, error) {
		listened++
		if language != "en" || hint == "" {
			t.Errorf("listener asked with language %q hint %q", language, hint)
		}
		return spoken("It", 3.2, 3.4, "won't", 3.4, 3.7, "fly", 3.7, 4.0, "I", 4.3, 4.4, "give", 4.4, 4.6, "up", 4.6, 4.9,
			"Wait", 7.0, 7.4, "I", 7.6, 7.7, "know", 7.7, 7.9, "a", 7.9, 8.0, "secret", 8.0, 8.6), 10, nil
	}
	out := make(true)
	for range 2 {
		cues, err := s.scriptCuesForClip(t.Context(), project, composition.Clip{ID: "c", AssetID: "output-" + out.ID, Start: 20, Duration: 10, Speed: 1})
		if err != nil || len(cues) != 2 || cues[0].Start != 23.12 || cues[0].End != 25.2 || cues[1].Start != 26.92 || cues[1].End != 28.9 {
			t.Fatalf("captions not at the heard times: %v %+v", err, cues)
		}
	}
	if listened != 1 {
		t.Fatalf("listened %d times, want once", listened)
	}
	entries, _ := s.database.ListCosts(project)
	if len(entries) != 1 || entries[0].Label != "Caption timing" || entries[0].Amount <= 0 || entries[0].Amount > 0.01 {
		t.Fatalf("cost: %+v", entries)
	}
	// A silent clip is never listened to: its voice is added, and timed, separately.
	silent := make(false)
	if cues, err := s.scriptCuesForClip(t.Context(), project, composition.Clip{ID: "s", AssetID: "output-" + silent.ID, Duration: 10, Speed: 1}); err != nil || len(cues) != 2 || listened != 1 {
		t.Fatalf("silent clip: %v %+v listened=%d", err, cues, listened)
	}
}
