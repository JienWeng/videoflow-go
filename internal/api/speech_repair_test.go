package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"videoflow-go/internal/agents"
	"videoflow-go/internal/models"
)

// repairFixture is a scene made in three clips (one per shot) whose third clip the
// speech check heard cut off: Mei's line l1 is in shot 1, and l2 (Mei) and l3 (the
// narrator) are in shot 3.
func repairFixture(t *testing.T) (*Server, *models.Scene, *models.RenderOutput, *models.Character) {
	t.Helper()
	s, scene := productionFixture(t, "shots")
	project := *scene.ProjectID
	mei := &models.Character{ProjectID: &project, Name: "Mei", Appearance: "Young woman"}
	if e := s.database.CreateCharacter(mei); e != nil {
		t.Fatal(e)
	}
	shots, _ := s.database.ListShots(scene.ID)
	scene.CharacterIDsJSON = voiceJSON([]string{mei.ID})
	scene.SceneJSON = voiceJSON(map[string]any{
		"production":       productionSettings{AudioMode: "native", Strategy: "shots"},
		"generation_brief": map[string]any{"language": "Chinese"},
		"spoken_lines": []SpokenLine{
			{ID: "l1", CharacterID: mei.ID, Language: "Chinese", Visible: true, Text: "钱是什么？", ShotID: shots[0].ID},
			{ID: "l2", CharacterID: mei.ID, Language: "Chinese", Visible: true, Text: "守住这台机器，不要让任何人靠近。", ShotID: shots[2].ID},
			{ID: "l3", Language: "Chinese", Text: "他从一万年后回来了。", ShotID: shots[2].ID},
		},
	})
	if e := s.database.UpdateScene(scene); e != nil {
		t.Fatal(e)
	}
	scene, _ = s.database.GetScene(scene.ID)
	return s, scene, flaggedSpeechClip(t, s, scene, shots[2].ID, 2, 3, "speech_cut_off"), mei
}

// flaggedSpeechClip saves a finished clip of the scene (unit index of count, made of
// shot) whose speech check found code.
func flaggedSpeechClip(t *testing.T, s *Server, scene *models.Scene, shot string, index, count int, code string) *models.RenderOutput {
	t.Helper()
	unit := map[string]any{"key": fmt.Sprintf("old:%d", index), "index": index, "count": count, "length": 4, "duration": 4, "prompt": "Mei stands guard"}
	if shot != "" {
		unit["shot_ids"] = []string{shot}
	}
	request, _ := json.Marshal(map[string]any{"generate_audio": true, "render_plan_id": "old", "render_unit": unit})
	job := &models.RenderJob{ProjectID: scene.ProjectID, SceneID: &scene.ID, Provider: "gemini", Status: models.StatusSucceeded, RequestJSON: request}
	if err := s.database.CreateRenderJob(job); err != nil {
		t.Fatal(err)
	}
	end := 3.9
	verdict := verdictFix
	if code == "wrong_language" || code == "no_speech" {
		verdict = verdictFail
	}
	record := selfCheckRecord{Kind: selfCheckClipAudio, Result: selfCheckResult{Verdict: verdict, Score: 55,
		Issues: []selfCheckIssue{{Code: code, Severity: "high", Detail: "Speech cut off at 3.9 s: line 1 stops before its end (heard “守住这台机器不要”)."}},
		Retry:  selfCheckRetry{WorthIt: true, Change: "Give line 1 fewer words so the speech is finished by about 3.4 s."},
		Audio:  &audioCheck{ClipLength: 4, SpeechEnd: &end, CutOff: true}}, Hash: "h", Attempt: 1}
	qa, _ := json.Marshal(map[string]any{"audio_check": record})
	out := &models.RenderOutput{RenderJobID: job.ID, VideoPath: "storage/outputs/" + job.ID + ".mp4", QAJSON: qa}
	if err := s.database.CreateRenderOutput(out); err != nil {
		t.Fatal(err)
	}
	return out
}

// writerSays makes the local session answer the speech repair with lines, counting the
// turns and keeping what it was asked.
func writerSays(t *testing.T, s *Server, lines *string) (*int, *string) {
	t.Helper()
	calls, asked := 0, ""
	s.agents.SetStructuredRunner(func(_ context.Context, name, _, prompt string, target any) error {
		calls++
		asked = prompt
		if name != "speech_repair_agent" {
			t.Errorf("unexpected agent %s", name)
		}
		return json.Unmarshal([]byte(`{"lines":`+*lines+`}`), target)
	})
	return &calls, &asked
}

// A repair rewrites only the flagged clip's lines, shorter, with their speakers and
// shots, from what the check heard and a tighter target; nothing is saved, and the
// price of the new clip comes with it.
func TestSpeechRepairRewritesOnlyThatClipsLines(t *testing.T) {
	s, scene, out, mei := repairFixture(t)
	answer := `[{"id":"l2","character_id":"` + mei.ID + `","text":"守住机器！","shot":1},{"id":"l3","character_id":"","text":"他回来了。","shot":1}]`
	calls, asked := writerSays(t, s, &answer)
	w := call(t, s, "POST", "/outputs/"+out.ID+"/speech-repair", "")
	if w.Code != 200 {
		t.Fatalf("repair: %d %s", w.Code, w.Body.String())
	}
	var repair speechRepair
	if err := json.Unmarshal(w.Body.Bytes(), &repair); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 || strings.Contains(*asked, `"l1"`) || !strings.Contains(*asked, `"id":"l2"`) || !strings.Contains(*asked, "Speech cut off at 3.9 s") || !strings.Contains(*asked, "the speech ended at 3.9 s") {
		t.Fatalf("asked %d: %s", *calls, *asked)
	}
	// Shot 3 is 3 s: 60-75 % of it in Chinese, never more than 75 %.
	if !strings.Contains(*asked, `"seconds":3`) || strings.Contains(*asked, `"seconds":2`) || !strings.Contains(*asked, `"target_length":"7-9 characters"`) || !strings.Contains(*asked, `"holds_at_most":"9 characters"`) {
		t.Fatalf("target: %s", *asked)
	}
	shots, _ := s.database.ListShots(scene.ID)
	if len(repair.After) != 3 || repair.After[0] != repair.Before[0] || strings.Join(repair.LineIDs, ",") != "l2,l3" {
		t.Fatalf("after: %+v", repair.After)
	}
	for i, want := range []string{"守住机器！", "他回来了。"} {
		l := repair.After[i+1]
		if l.Text != want || l.ShotID != shots[2].ID || l.CharacterID != repair.Before[i+1].CharacterID || l.Visible != repair.Before[i+1].Visible {
			t.Fatalf("line %d: %+v", i+2, l)
		}
	}
	if repair.Quote.PlanID == "" || repair.Quote.Seconds <= 0 || repair.Quote.Calls <= 0 || repair.ClipLength != 4 || *repair.SpeechEnd != 3.9 {
		t.Fatalf("quote %+v", repair)
	}
	if again, _ := s.database.GetScene(scene.ID); !sameLines(sceneSpokenLines(again), repair.Before) {
		t.Fatal("a proposal must not be saved")
	}
	if jobs, _ := s.database.ListRenderJobs(*scene.ProjectID); len(jobs) != 1 {
		t.Fatalf("a proposal must not make a video: %d jobs", len(jobs))
	}

	// Answers that touch another clip, change who speaks, move a line or make it longer
	// are refused, and nothing changes.
	for name, bad := range map[string]string{
		"another clip's line": `[{"id":"l1","character_id":"` + mei.ID + `","text":"钱？","shot":1},{"id":"l2","character_id":"` + mei.ID + `","text":"守住！","shot":1},{"id":"l3","character_id":"","text":"他回来了。","shot":1}]`,
		"another speaker":     `[{"id":"l2","character_id":"","text":"守住机器！","shot":1},{"id":"l3","character_id":"","text":"他回来了。","shot":1}]`,
		"a longer line":       `[{"id":"l2","character_id":"` + mei.ID + `","text":"守住机器！","shot":1},{"id":"l3","character_id":"","text":"他从一万年以后的世界回来了啊。","shot":1}]`,
		"a dropped line":      `[{"id":"l2","character_id":"` + mei.ID + `","text":"守住机器！","shot":1}]`,
	} {
		answer = bad
		if w = call(t, s, "POST", "/outputs/"+out.ID+"/speech-repair", ""); w.Code != 422 || !strings.Contains(w.Body.String(), "can't be used") {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
}

// Applying a repair saves the lines with a revision and makes the scene's clip again
// at the price shown, once; a stale proposal or a higher price saves nothing.
func TestSpeechRepairApplySavesThenRendersOnce(t *testing.T) {
	s, scene, out, mei := repairFixture(t)
	answer := `[{"id":"l2","character_id":"` + mei.ID + `","text":"守住机器！","shot":1},{"id":"l3","character_id":"","text":"他回来了。","shot":1}]`
	writerSays(t, s, &answer)
	renders := fakeRepairRender(t)
	repair, err := s.proposeSpeechRepair(context.Background(), *scene.ProjectID, out.ID)
	if err != nil {
		t.Fatal(err)
	}
	cheaper := repair.Quote
	cheaper.Seconds--
	for name, body := range map[string]speechRepairApply{
		"stale":   {After: repair.After, ExpectedUpdatedAt: repair.ExpectedUpdatedAt.Add(-1), Quote: repair.Quote},
		"pricier": {After: repair.After, ExpectedUpdatedAt: repair.ExpectedUpdatedAt, Quote: cheaper},
		"edited":  {After: repair.Before, ExpectedUpdatedAt: repair.ExpectedUpdatedAt, Quote: repair.Quote},
	} {
		raw, _ := json.Marshal(body)
		if w := call(t, s, "POST", "/outputs/"+out.ID+"/speech-repair/apply", string(raw)); w.Code < 400 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	if len(*renders) != 0 {
		t.Fatal("a refused repair made a video")
	}
	raw, _ := json.Marshal(speechRepairApply{After: repair.After, ExpectedUpdatedAt: repair.ExpectedUpdatedAt, Quote: repair.Quote})
	if w := call(t, s, "POST", "/outputs/"+out.ID+"/speech-repair/apply", string(raw)); w.Code != 200 || !strings.Contains(w.Body.String(), `"job_ids":["fake-job"]`) {
		t.Fatalf("apply: %d %s", w.Code, w.Body.String())
	}
	saved, _ := s.database.GetScene(scene.ID)
	if !sameLines(sceneSpokenLines(saved), repair.After) || len(*renders) != 1 || (*renders)[0] != scene.ID+"|"+repair.Quote.PlanID {
		t.Fatalf("saved %+v renders %v", sceneSpokenLines(saved), *renders)
	}
	if revisions, _ := s.database.ListRevisions("scene", scene.ID); len(revisions) == 0 {
		t.Fatal("no revision of the old lines")
	}
}

// fakeRepairRender stands in for starting the scene's video: it records each start.
func fakeRepairRender(t *testing.T) *[]string {
	t.Helper()
	started := []string{}
	old := speechRepairRender
	t.Cleanup(func() { speechRepairRender = old })
	speechRepairRender = func(_ *Server, _ context.Context, sceneID string, quote speechRepairQuote) ([]string, error) {
		started = append(started, sceneID+"|"+quote.PlanID)
		return []string{"fake-job"}, nil
	}
	return &started
}

// Wrong language and no speech are never repaired by a shorter line.
func TestSpeechRepairRefusesWhatAShorterLineCannotFix(t *testing.T) {
	s, scene, _, _ := repairFixture(t)
	shots, _ := s.database.ListShots(scene.ID)
	calls := 0
	s.agents.SetStructuredRunner(func(context.Context, string, string, string, any) error { calls++; return nil })
	for _, code := range []string{"wrong_language", "no_speech"} {
		out := flaggedSpeechClip(t, s, scene, shots[2].ID, 2, 3, code)
		if w := call(t, s, "POST", "/outputs/"+out.ID+"/speech-repair", ""); w.Code != 422 || !strings.Contains(w.Body.String(), "won't fix that") {
			t.Fatalf("%s: %d %s", code, w.Code, w.Body.String())
		}
	}
	if calls != 0 {
		t.Fatal("the writer was asked anyway")
	}
}

// The chat turns repair_clip_speech into a card for a flagged clip; a clip a shorter
// line can't fix, or no flagged clip, gets a reply saying so instead.
func TestRepairClipSpeechIsAChatAction(t *testing.T) {
	if canonicalChatAction(speechRepairAction) != speechRepairAction || !selectedChatActionAllowed("output", speechRepairAction) {
		t.Fatal("repair_clip_speech is not a chat action")
	}
	if !strings.Contains(chatMode(true, false), "so does repair_clip_speech") || !strings.Contains(chatMode(true, true), "repair_clip_speech also starts by itself, but only once per clip") {
		t.Fatal("assistant modes do not say how repair_clip_speech starts")
	}
	cut := selfCheckFlag{Kind: selfCheckClipAudio, ID: "o1", SceneID: "s1", Label: "“Guard” clip’s speech", Verdict: verdictFix, Issues: []selfCheckIssue{{Code: "speech_cut_off", Detail: "Speech cut off at 7.9 s."}}}
	wrong := selfCheckFlag{Kind: selfCheckClipAudio, ID: "o2", SceneID: "s2", Label: "“Chase” clip’s speech", Verdict: verdictFail, Issues: []selfCheckIssue{{Code: "wrong_language", Detail: "Omni spoke English."}}}
	for _, tc := range []struct {
		name, output, want string
		flags              []selfCheckFlag
	}{
		{name: "the first flagged clip", flags: []selfCheckFlag{cut, wrong}, want: "o1"},
		{name: "the named clip", output: "o1", flags: []selfCheckFlag{wrong, cut}, want: "o1"},
		{name: "wrong language", output: "o2", flags: []selfCheckFlag{cut, wrong}, want: "won't fix that"},
		{name: "nothing flagged", want: "hasn't flagged any clip"},
	} {
		turn := &chatTurn{selfChecks: tc.flags}
		intent := map[string]any{"action": speechRepairAction}
		action := resolveChatSpeechRepair(turn, &agents.ChatIntentResponse{OutputID: tc.output}, speechRepairAction, intent)
		if tc.want == "o1" && (action != speechRepairAction || intent["output_id"] != "o1" || intent["scene_id"] != "s1") {
			t.Fatalf("%s: %s %+v", tc.name, action, intent)
		}
		if tc.want != "o1" && (action != "unknown" || !strings.Contains(intent["reply"].(string), tc.want)) {
			t.Fatalf("%s: %s %+v", tc.name, action, intent)
		}
	}
	items := chatSelfCheck([]selfCheckFlag{cut, wrong})["flagged"].([]map[string]any)
	if items[0]["repairable"] != true || items[1]["repairable"] != false || items[0]["decision_key"] != "selfcheck:o1" {
		t.Fatalf("%+v", items)
	}
}
