package api

import (
	"encoding/json"
	"strings"
	"testing"

	"videoflow-go/internal/models"
	"videoflow-go/internal/speech"
)

// The real project's scene: 8 seconds in three shots (3 s, 2 s, 3 s), Chinese, two
// short lines saved for the whole scene.
func speechScene(lines []SpokenLine, production map[string]any) (*models.Scene, []models.Shot) {
	doc := map[string]any{"spoken_lines": lines, "generation_brief": map[string]any{"language": "Chinese"}}
	if production != nil {
		doc["production"] = production
	}
	scene := &models.Scene{ID: "scene-1", Title: "Trust", Duration: 8, SceneJSON: voiceJSON(doc)}
	shots := []models.Shot{{ID: "shot-c", ShotOrder: 3, Duration: 3}, {ID: "shot-a", ShotOrder: 1, Duration: 3}, {ID: "shot-b", ShotOrder: 2, Duration: 2}}
	return scene, shots
}

func TestSpeechPlanPlacesUnassignedLinesByTime(t *testing.T) {
	lines := []SpokenLine{
		{ID: "l1", CharacterID: "c1", Language: "Chinese", Text: "如果钱不需要靠银行来信任呢？"}, // 13 chars ≈ 3.1 s
		{ID: "l2", CharacterID: "c2", Language: "zh", Text: "那就用代码。"},              // 5 chars ≈ 1.2 s
	}
	scene, shots := speechScene(lines, nil)
	plan := planSpeech(scene, lines, shots, nil, map[string]string{"c1": "Mei", "c2": "Jun"}, nil)
	if len(plan.Shots) != 3 || plan.Shots[0].ShotID != "shot-a" || plan.Shots[2].Start != 5 {
		t.Fatalf("shots in order with their starts: %+v", plan.Shots)
	}
	// Line 1 starts in shot 1; line 2 starts when line 1 ends, in shot 2.
	if got := plan.Shots[0].Lines; len(got) != 1 || got[0].ID != "l1" || got[0].Placed != placedAuto || got[0].Speaker != "Mei" {
		t.Fatalf("shot 1 lines: %+v", got)
	}
	if got := plan.Shots[1].Lines; len(got) != 1 || got[0].ID != "l2" || got[0].Language != "Chinese" {
		t.Fatalf("shot 2 lines: %+v", got)
	}
	// Line 1 runs past its 3-second shot, but the clip as a whole has room.
	if plan.Shots[0].Verdict != speech.TooLong || plan.Clips[0].Verdict != speech.Tight || plan.Clips[0].ShotsOver != 1 {
		t.Fatalf("verdicts: shot %s clip %+v", plan.Shots[0].Verdict, plan.Clips[0])
	}
	if !strings.Contains(plan.Summary, "runs past its shot") {
		t.Fatalf("summary = %q", plan.Summary)
	}
	// Placing a line is a guess: nothing was saved on the lines.
	if sceneSpokenLines(scene)[0].ShotID != "" {
		t.Fatal("the guess must not be saved")
	}
}

func TestSpeechPlanHonoursChosenShotsAndClips(t *testing.T) {
	lines := []SpokenLine{
		{ID: "l1", Language: "English", Text: "What if money did not need a bank?", ShotID: "shot-c"},
		{ID: "l2", Language: "English", Text: "Then trust is code."},
	}
	scene, shots := speechScene(lines, map[string]any{"dialogue_shots": map[string]string{"l2": "shot-b"}})
	plan := planSpeech(scene, lines, shots, nil, nil, nil)
	if got := plan.Shots[2].Lines; len(got) != 1 || got[0].ID != "l1" || got[0].Placed != placedLine || got[0].Speaker != "Narrator" {
		t.Fatalf("shot 3: %+v", got)
	}
	if got := plan.Shots[1].Lines; len(got) != 1 || got[0].ID != "l2" || got[0].Placed != placedClip {
		t.Fatalf("shot 2: %+v", got)
	}
	// Shot 3's line (8 words, 3.2 s) runs past its 3 seconds, the end of the clip.
	if plan.Shots[2].Verdict != speech.TooLong || plan.Clips[0].Verdict != speech.TooLong || plan.Clips[0].Used != 8.2 {
		t.Fatalf("verdicts: %+v %+v", plan.Shots[2], plan.Clips[0])
	}
	// A shot that is not in the scene (deleted) leaves the line to be placed by time.
	lines[0].ShotID = "gone"
	scene, _ = speechScene(lines, nil)
	plan = planSpeech(scene, lines, shots, nil, nil, nil)
	if got := plan.Shots[0].Lines; len(got) != 1 || got[0].Placed != placedAuto {
		t.Fatalf("stale shot: %+v", plan.Shots)
	}
}

func TestSpeechPlanPerClip(t *testing.T) {
	long := "钱是什么？谁来保管它？如果钱不需要靠银行来信任，我们每个人都能自己检查每一笔账，那会怎样呢？"
	lines := []SpokenLine{{ID: "l1", Language: "zh", Text: long, ShotID: "shot-a"}, {ID: "l2", Language: "zh", Text: "开始吧。", ShotID: "shot-c"}}
	scene, shots := speechScene(lines, nil)
	units := []renderUnit{{ShotIDs: []string{"shot-a", "shot-b"}, Length: 5}, {ShotIDs: []string{"shot-c"}, Length: 3}}
	plan := planSpeech(scene, lines, shots, units, nil, nil)
	if len(plan.Clips) != 2 || plan.Clips[0].Verdict != speech.TooLong || plan.Clips[1].Verdict != speech.OK {
		t.Fatalf("clips: %+v", plan.Clips)
	}
	if plan.Verdict != speech.TooLong || !strings.HasPrefix(plan.Summary, "Clip 1 · Won't fit: ") || !strings.Contains(plan.Summary, "of 5 s — fit the script first") {
		t.Fatalf("summary = %q", plan.Summary)
	}
	if !strings.Contains(plan.Clips[1].Summary, "Speech fits: ") || !strings.HasSuffix(plan.Clips[1].Summary, "of 3 s ✓") {
		t.Fatalf("clip 2 summary = %q", plan.Clips[1].Summary)
	}
	// A scene with no shots yet is fitted as a whole.
	plan = planSpeech(scene, lines[1:], nil, nil, nil, nil)
	if len(plan.Shots) != 1 || plan.Shots[0].Lines[0].Placed != placedScene || plan.Clips[0].Seconds != 8 || plan.Verdict != speech.OK {
		t.Fatalf("whole scene: %+v", plan)
	}
	// A silent scene has no verdict.
	if plan = planSpeech(scene, nil, shots, nil, nil, nil); plan.Verdict != "" || plan.Summary != "" || plan.Lines != 0 {
		t.Fatalf("silent: %+v", plan)
	}
}

// PUT /scenes/{id}/spoken-lines keeps a line's shot only when it is one of the scene's
// shots, and its start hint inside it; the speech fit endpoint reads saved and draft lines.
func TestSpokenLinesShotsAreValidatedAndFitted(t *testing.T) {
	s, scene := productionFixture(t, "auto")
	shots, _ := s.database.ListShots(scene.ID)
	other := renderableScene(t, s, nil, "Other", 2, "auto")
	otherShots, _ := s.database.ListShots(other.ID)
	path := "/scenes/" + scene.ID + "/spoken-lines"
	for _, bad := range []map[string]any{
		{"id": "l1", "language": "Chinese", "text": "你好", "shot_id": otherShots[0].ID},
		{"id": "l1", "language": "Chinese", "text": "你好", "shot_id": shots[0].ID, "start_hint": 3.0},
		{"id": "l1", "language": "Chinese", "text": "你好", "start_hint": -1.0},
	} {
		if w := call(t, s, "PUT", path, spokenLinesBody(t, scene, []map[string]any{bad})); w.Code != 400 {
			t.Fatalf("%v saved: %d %s", bad, w.Code, w.Body.String())
		}
	}
	hint := 0.5
	body := spokenLinesBody(t, scene, []map[string]any{
		{"id": "l1", "language": "Chinese", "text": "如果钱不需要靠银行来信任呢？", "shot_id": shots[0].ID, "start_hint": hint},
		{"id": "l2", "language": "Chinese", "text": "那我们就用代码来建立信任。"},
	})
	if w := call(t, s, "PUT", path, body); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	saved, _ := s.database.GetScene(scene.ID)
	lines := sceneSpokenLines(saved)
	if lines[0].ShotID != shots[0].ID || lines[0].StartHint == nil || *lines[0].StartHint != hint || lines[1].ShotID != "" {
		t.Fatalf("saved: %+v", lines)
	}
	// The line's own shot is the clip choice the rest of the app reads.
	if sceneProduction(saved).DialogueShots["l1"] != shots[0].ID {
		t.Fatalf("dialogue shots: %+v", sceneProduction(saved).DialogueShots)
	}

	w := call(t, s, "GET", "/scenes/"+scene.ID+"/speech-fit", "")
	var fit sceneSpeech
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &fit) != nil || fit.Lines != 2 || len(fit.Clips) == 0 || fit.Summary == "" {
		t.Fatalf("speech fit: %d %s", w.Code, w.Body.String())
	}
	if fit.Shots[0].Lines[0].Start != hint || fit.Shots[0].Lines[0].Placed != placedLine {
		t.Fatalf("shot 1: %+v", fit.Shots[0])
	}
	// A draft that is far too long is judged without being saved.
	draft := `{"lines":[{"id":"x","language":"English","text":"` + strings.Repeat("word ", 40) + `"}]}`
	w = call(t, s, "POST", "/scenes/"+scene.ID+"/speech-fit", draft)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &fit) != nil || fit.Verdict != speech.TooLong || !strings.Contains(fit.Summary, "Won't fit") {
		t.Fatalf("draft fit: %d %s", w.Code, w.Body.String())
	}
	if again, _ := s.database.GetScene(scene.ID); len(sceneSpokenLines(again)) != 2 {
		t.Fatal("a draft fit must not save")
	}
}
