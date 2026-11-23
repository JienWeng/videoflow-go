package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"videoflow-go/internal/models"
	"videoflow-go/internal/speech"
)

// fitScene is the real project's shape: an 8-second Chinese scene in shots of 3, 2 and
// 3 seconds, with one line far too long for its shot.
func fitScene(t *testing.T) (*Server, *models.Scene, []models.Shot, *models.Character) {
	t.Helper()
	s, scene := productionFixture(t, "auto")
	project := *scene.ProjectID
	mei := &models.Character{ProjectID: &project, Name: "Mei", Appearance: "Young woman"}
	if e := s.database.CreateCharacter(mei); e != nil {
		t.Fatal(e)
	}
	shots, _ := s.database.ListShots(scene.ID)
	scene.CharacterIDsJSON = voiceJSON([]string{mei.ID})
	scene.SceneJSON = voiceJSON(map[string]any{
		"production":       productionSettings{AudioMode: "native", Strategy: "auto"},
		"generation_brief": map[string]any{"language": "Chinese"},
		"spoken_lines": []SpokenLine{
			{ID: "l1", CharacterID: mei.ID, Language: "Chinese", Visible: true, Text: "钱是什么？谁来保管它？如果钱不需要靠银行来信任，我们每个人都能自己检查每一笔账，那会怎样呢？", ShotID: shots[0].ID},
		},
	})
	if e := s.database.UpdateScene(scene); e != nil {
		t.Fatal(e)
	}
	scene, _ = s.database.GetScene(scene.ID)
	return s, scene, shots, mei
}

// Fit script to clips asks the local session once and returns before and after without
// saving; applying the proposal is a normal spoken-script save.
func TestFitScriptProposesShotSizedLines(t *testing.T) {
	s, scene, shots, mei := fitScene(t)
	calls := 0
	var facts string
	s.agents.SetStructuredRunner(func(_ context.Context, name, _, prompt string, target any) error {
		calls++
		facts = prompt
		if name != "script_fit_agent" {
			t.Errorf("unexpected agent %s", name)
		}
		return json.Unmarshal([]byte(`{"lines":[`+
			`{"id":"l1","character_id":"`+mei.ID+`","text":"钱是什么？谁来保管它？","shot":1},`+
			`{"id":"l1_b","character_id":"`+mei.ID+`","text":"不靠银行呢？","shot":2},`+
			`{"id":"l2","character_id":"","text":"每个人都能自己查账。","shot":3}]}`), target)
	})
	w := call(t, s, "POST", "/scenes/"+scene.ID+"/spoken-lines/fit", "")
	if w.Code != 200 {
		t.Fatalf("fit: %d %s", w.Code, w.Body.String())
	}
	var fit scriptFit
	if err := json.Unmarshal(w.Body.Bytes(), &fit); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !strings.Contains(facts, `"holds_at_most":"12 characters"`) || !strings.Contains(facts, `"target_length":"8-11 characters"`) {
		t.Fatalf("calls %d, facts %s", calls, facts)
	}
	if fit.BeforeFit.Verdict != speech.TooLong || fit.AfterFit.Verdict == speech.TooLong || len(fit.Before) != 1 || len(fit.After) != 3 {
		t.Fatalf("before %s after %s: %+v", fit.BeforeFit.Verdict, fit.AfterFit.Verdict, fit.After)
	}
	for i, want := range []string{shots[0].ID, shots[1].ID, shots[2].ID} {
		if fit.After[i].ShotID != want || fit.After[i].Language != "Chinese" {
			t.Fatalf("line %d: %+v", i, fit.After[i])
		}
	}
	if !fit.After[1].Visible || fit.After[2].Visible {
		t.Fatalf("on-screen flags: %+v", fit.After)
	}
	if again, _ := s.database.GetScene(scene.ID); len(sceneSpokenLines(again)) != 1 {
		t.Fatal("a proposal must not be saved")
	}
	body, _ := json.Marshal(map[string]any{"lines": fit.After, "expected_updated_at": fit.ExpectedUpdatedAt})
	if w = call(t, s, "PUT", "/scenes/"+scene.ID+"/spoken-lines", string(body)); w.Code != 200 {
		t.Fatalf("apply: %d %s", w.Code, w.Body.String())
	}
	if saved, _ := s.database.GetScene(scene.ID); len(sceneSpokenLines(saved)) != 3 {
		t.Fatal("applied lines not saved")
	}

	// An answer that adds a speaker is refused, and nothing changes.
	s.agents.SetStructuredRunner(func(_ context.Context, _, _, _ string, target any) error {
		return json.Unmarshal([]byte(`{"lines":[{"id":"x","character_id":"stranger","text":"你好","shot":1}]}`), target)
	})
	if w = call(t, s, "POST", "/scenes/"+scene.ID+"/spoken-lines/fit", ""); w.Code != 422 {
		t.Fatalf("bad answer: %d %s", w.Code, w.Body.String())
	}
}

// Full auto and a new story save the fit themselves, unless it is still too long and no
// shorter than before.
func TestFitSceneScriptSavesOnlyAnImprovement(t *testing.T) {
	s, scene, _, mei := fitScene(t)
	answer := `{"lines":[{"id":"l1","character_id":"` + mei.ID + `","text":"` + strings.Repeat("钱是什么", 12) + `","shot":1}]}`
	s.agents.SetStructuredRunner(func(_ context.Context, _, _, _ string, target any) error {
		return json.Unmarshal([]byte(answer), target)
	})
	plan, err := s.fitSceneScript(context.Background(), scene)
	if err != nil || plan.Verdict != speech.TooLong {
		t.Fatalf("longer rewrite: %v %+v", err, plan)
	}
	if again, _ := s.database.GetScene(scene.ID); !strings.HasPrefix(sceneSpokenLines(again)[0].Text, "钱是什么？谁来保管它？") {
		t.Fatal("a longer rewrite was saved")
	}
	answer = `{"lines":[{"id":"l1","character_id":"` + mei.ID + `","text":"钱是什么？","shot":1}]}`
	if plan, err = s.fitSceneScript(context.Background(), scene); err != nil || plan.Verdict == speech.TooLong {
		t.Fatalf("fitting rewrite: %v %+v", err, plan)
	}
	if again, _ := s.database.GetScene(scene.ID); sceneSpokenLines(again)[0].Text != "钱是什么？" {
		t.Fatal("the fitted rewrite was not saved")
	}
}

// Planning shots gives the planner the lines and saves each line with the new shot it
// is spoken in, together with the shots.
func TestGeneratedShotsCarryTheirLines(t *testing.T) {
	s, scene, oldShots, _ := fitScene(t)
	var prompt string
	s.agents.SetStructuredRunner(func(_ context.Context, _, _, user string, target any) error {
		prompt = user
		return json.Unmarshal([]byte(`{"shots":[{"duration":5,"prompt":"Mei talks","spoken_line_ids":["l1"]},{"duration":3,"prompt":"Street"}]}`), target)
	})
	w := call(t, s, "POST", "/scenes/"+scene.ID+"/shots/generate?background=true", `{}`)
	var reply struct {
		ID string `json:"op_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &reply)
	deadline := time.Now().Add(5 * time.Second)
	for {
		op, _ := s.database.GetOp(reply.ID)
		if op != nil && (op.Status == "succeeded" || op.Status == "failed") {
			if op.Status != "succeeded" {
				t.Fatal(op.Error)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("shot generation did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(prompt, `"id":"l1"`) {
		t.Fatalf("planner was not given the lines: %s", prompt)
	}
	shots, _ := s.database.ListShots(scene.ID)
	saved, _ := s.database.GetScene(scene.ID)
	line := sceneSpokenLines(saved)[0]
	if len(shots) != 2 || line.ShotID != shots[0].ID || line.ShotID == oldShots[0].ID {
		t.Fatalf("line shot %q, shots %+v", line.ShotID, shots)
	}
}

func TestFitScriptIsASceneChatAction(t *testing.T) {
	if canonicalChatAction("fit_script") != "fit_script" || !sceneScopedAction("fit_script") {
		t.Fatal("fit_script is not a scene chat action")
	}
	if !strings.Contains(chatMode(true, true), "fit_script start by themselves") || !strings.Contains(chatMode(true, false), "so does fit_script") {
		t.Fatal("assistant modes do not say how fit_script starts")
	}
}
