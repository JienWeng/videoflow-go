package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestScriptSpeechRulesUseTheLanguagesUnit(t *testing.T) {
	zh := ScriptSpeechRules("Chinese")
	for _, want := range []string{"4.2 characters a second", "an 8-second scene holds 23-30 characters", "70-90%", "at most about 11 characters", "Do not write one short line for a whole scene"} {
		if !strings.Contains(zh, want) {
			t.Errorf("Chinese rules lack %q: %s", want, zh)
		}
	}
	if en := ScriptSpeechRules("English"); !strings.Contains(en, "2.5 words a second") || !strings.Contains(en, "14-18 words") {
		t.Errorf("English rules: %s", en)
	}
	if es := ScriptSpeechRules("Spanish"); !strings.Contains(es, "2.6 words a second") {
		t.Errorf("Spanish rules: %s", es)
	}
}

// The script writer is told the budget; a scene that still cannot be said in its seconds
// gets exactly one shortening turn, kept only when it is shorter.
func TestGenerateScriptShortensOverlongScenesOnce(t *testing.T) {
	long := strings.Repeat("如果钱不需要靠银行来信任呢？", 4) // ≈12.4 s
	script := `{"title":"Trust","summary":"s","scenes":[` +
		`{"title":"One","summary":"a","duration":8,"spoken_lines":[{"id":"l1","character_id":"","language":"Chinese","text":"` + long + `"}]},` +
		`{"title":"Two","summary":"b","duration":8,"spoken_lines":[{"id":"l2","character_id":"","language":"Chinese","text":"那就用代码。"}]}]}`
	calls, systems, facts := 0, []string{}, []string{}
	engine := NewAgentEngine()
	engine.SetStructuredRunner(func(_ context.Context, name, system, prompt string, target any) error {
		calls++
		systems, facts = append(systems, system), append(facts, prompt)
		if name == "script_fit_agent" {
			return json.Unmarshal([]byte(`{"scenes":[{"scene":1,"spoken_lines":[{"id":"l1","character_id":"","language":"Chinese","text":"如果钱不需要靠银行来信任呢？那就自己检查每一笔账。"}]}]}`), target)
		}
		return json.Unmarshal([]byte(script), target)
	})
	draft, err := engine.GenerateScript(WithDialogueLanguage(context.Background(), "Chinese"), "trust", 2, 16, "9:16")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(systems[0], "an 8-second scene holds 23-30 characters") {
		t.Fatalf("writer not told the budget: %s", systems[0])
	}
	if calls != 2 || !strings.Contains(facts[1], `"scene":1`) || strings.Contains(facts[1], `"scene":2`) {
		t.Fatalf("calls = %d, shortening facts = %v", calls, facts)
	}
	if got := draft.Scenes[0].SpokenLines[0].Text; got != "如果钱不需要靠银行来信任呢？那就自己检查每一笔账。" {
		t.Fatalf("scene 1 not shortened: %q", got)
	}
	if draft.Scenes[1].SpokenLines[0].Text != "那就用代码。" {
		t.Fatal("a scene that fits must not change")
	}

	// A "shortened" answer that is longer, or gives a line to a new speaker, is ignored.
	for _, bad := range []string{
		`{"scenes":[{"scene":1,"spoken_lines":[{"id":"l1","character_id":"","language":"Chinese","text":"` + long + long + `"}]}]}`,
		`{"scenes":[{"scene":1,"spoken_lines":[{"id":"l1","character_id":"stranger","language":"Chinese","text":"短。"}]}]}`,
	} {
		engine.SetStructuredRunner(func(_ context.Context, name, _, _ string, target any) error {
			if name == "script_fit_agent" {
				return json.Unmarshal([]byte(bad), target)
			}
			return json.Unmarshal([]byte(script), target)
		})
		draft, _ = engine.GenerateScript(WithDialogueLanguage(context.Background(), "Chinese"), "trust", 2, 16, "9:16")
		if draft.Scenes[0].SpokenLines[0].Text != long {
			t.Fatalf("bad rewrite kept: %q", draft.Scenes[0].SpokenLines[0].Text)
		}
	}
}

func TestGenerateShotsPlacesEachLineOnce(t *testing.T) {
	engine := NewAgentEngine()
	var prompt string
	engine.SetStructuredRunner(func(_ context.Context, _, system, user string, target any) error {
		prompt = system + user
		return json.Unmarshal([]byte(`{"shots":[{"duration":3,"prompt":"Mei asks","spoken_line_ids":["l1","ghost"]},{"duration":2,"prompt":"Jun answers","spoken_line_ids":["l2","l1"]},{"duration":3,"prompt":"They walk"}]}`), target)
	})
	shots, err := engine.GenerateShots(context.Background(), "Trust", "s", 8,
		ShotLine{ID: "l1", Speaker: "Mei", Text: "如果钱不需要靠银行来信任呢？", Seconds: 3.1},
		ShotLine{ID: "l2", Speaker: "Jun", Text: "那就用代码。", Seconds: 1.2})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(shots[0].LineIDs, ",") != "l1" || strings.Join(shots[1].LineIDs, ",") != "l2" || len(shots[2].LineIDs) != 0 {
		t.Fatalf("line ids: %+v", shots)
	}
	if !strings.Contains(prompt, "spoken_line_ids") || !strings.Contains(prompt, `"seconds_to_say":3.1`) {
		t.Fatalf("planner not given the lines: %s", prompt)
	}
}

func TestFitScriptRefusesUnknownSpeakersAndShots(t *testing.T) {
	engine := NewAgentEngine()
	shots := []FitShot{{Number: 1, Seconds: 3}, {Number: 2, Seconds: 5}}
	speakers := map[string]string{"": "Narrator", "c1": "Mei"}
	for answer, want := range map[string]string{
		`{"lines":[{"id":"l1","character_id":"c9","text":"你好","shot":1}]}`: "not in the scene",
		`{"lines":[{"id":"l1","character_id":"c1","text":"你好","shot":3}]}`: "does not have",
		`{"lines":[{"id":"l1","character_id":"c1","text":" ","shot":1}]}`:  "empty",
		`{"lines":[]}`: "no spoken lines",
	} {
		engine.SetStructuredRunner(func(_ context.Context, _, _, _ string, target any) error {
			return json.Unmarshal([]byte(answer), target)
		})
		if _, err := engine.FitScript(context.Background(), "t", "s", "Chinese", shots, []FitLine{{ID: "l1", CharacterID: "c1", Text: "你好", Shot: 1}}, speakers); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", answer, err, want)
		}
	}
}
