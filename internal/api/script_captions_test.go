package api

import (
	"encoding/json"
	"strings"
	"testing"

	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
)

func TestPlaceScriptLinesUsesShotsAndTextLength(t *testing.T) {
	shots := []shotSpan{{ID: "a", Start: 0, End: 2}, {ID: "b", Start: 2, End: 6}}
	lines := []SpokenLine{{ID: "1", Text: "Hi"}, {ID: "2", Text: "A much longer second line"}, {ID: "3", Text: "Elsewhere"}}
	got := placeScriptLines(lines, map[string]string{"1": "a", "2": "b", "3": "other-clip"}, shots, 6, true)
	if len(got) != 2 || got[0].ID != "1" || got[1].ID != "2" {
		t.Fatalf("%+v", got)
	}
	if got[0].Start < 0 || got[0].End > 2 || got[1].Start < 2 || got[1].End > 6 {
		t.Fatalf("lines left their shots: %+v", got)
	}
	// Unassigned lines share the clip by length, longer lines get more time.
	shared := placeScriptLines([]SpokenLine{{ID: "x", Text: "Short"}, {ID: "y", Text: "This one is clearly a lot longer"}}, nil, shots, 6, true)
	if len(shared) != 2 || shared[1].End-shared[1].Start <= shared[0].End-shared[0].Start || shared[0].End > shared[1].Start {
		t.Fatalf("%+v", shared)
	}
	// In one clip of a multi-clip scene, unassigned lines are ambiguous and skipped.
	// A short line is shown for reading time, centered in its slot, not stretched over it.
	short := placeScriptLines([]SpokenLine{{ID: "s", Text: "Sit!"}}, nil, []shotSpan{{ID: "a", Start: 0, End: 5}}, 5, true)
	if len(short) != 1 || short[0].End-short[0].Start > 1.3 || short[0].Start < 1.5 || short[0].End > 3.5 {
		t.Fatalf("%+v", short)
	}
	if len(placeScriptLines([]SpokenLine{{ID: "x", Text: "Hello"}}, nil, shots, 6, false)) != 0 {
		t.Fatal("unassigned line placed in a partial clip")
	}
}

func TestScriptCaptionsFollowClipOnTimeline(t *testing.T) {
	s := routerServer(t, nil)
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

	// Clip placed at 10s on the timeline with the first second trimmed off.
	clip := composition.Clip{ID: "clip-1", AssetID: "output-" + out.ID, Start: 10, Trim: 1, Duration: 5, Speed: 1}
	cues, err := s.scriptCuesForClip(project, clip)
	if err != nil || len(cues) != 2 {
		t.Fatal(err, cues)
	}
	if cues[0].Start != 10 || cues[0].End > 11.0001 || cues[1].Start < 11 || cues[1].End > 15 || cues[0].SourceClipID != "" {
		t.Fatalf("not mapped onto the timeline: %+v", cues)
	}

	body, _ := json.Marshal(map[string]any{"clips": []any{clip, composition.Clip{ID: "img", AssetID: "asset_x", Start: 0, Duration: 3, Speed: 1}}})
	w := call(t, s, "POST", "/compositions/captions/from-script", string(body))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Let's go back in time!") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call(t, s, "GET", "/outputs/"+out.ID+"/captions/from-script", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"segments":[{`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
