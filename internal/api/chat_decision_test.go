package api

import (
	"encoding/json"
	"strings"
	"testing"

	"videoflow-go/internal/agents"
)

func TestChatDecisionIsValidatedAndDefaultsSafely(t *testing.T) {
	long := strings.Repeat("x", 500)
	for _, tc := range []struct {
		name   string
		raw    *agents.ChatDecision
		action string
		want   string
		check  func(t *testing.T, d chatDecision)
	}{
		{name: "missing with an action acts", action: "render_scene", want: decisionAct},
		{name: "missing without an action informs", action: "unknown", want: decisionInform},
		{name: "unknown mode informs", raw: &agents.ChatDecision{Mode: "maybe"}, action: "render_scene", want: decisionInform},
		{name: "act with nothing to run informs", raw: &agents.ChatDecision{Mode: "act"}, action: "unknown", want: decisionInform},
		{name: "a question without text informs", raw: &agents.ChatDecision{Mode: "ask", Options: []agents.DecisionOption{{Label: "A"}}}, action: "unknown", want: decisionInform},
		{name: "act drops question and options", raw: &agents.ChatDecision{Mode: "ACT", Question: "Which?", Options: []agents.DecisionOption{{Label: "A"}}}, action: "generate_shots", want: decisionAct, check: func(t *testing.T, d chatDecision) {
			if d.Question != "" || d.Options != nil {
				t.Fatalf("%+v", d)
			}
		}},
		{name: "ask is capped and cleaned", raw: &agents.ChatDecision{
			Mode: "ask", Question: "  Which format? ", Reason: "options", Key: "aspect_ratio",
			Assumptions: []string{" one ", "", "two", "three", "four", long},
			Options:     []agents.DecisionOption{{Label: "Vertical", Value: "9:16 please"}, {Label: "vertical"}, {Label: "Wide"}, {Label: ""}, {Label: "Square"}, {Label: "Cinema"}, {Label: "Sixth"}},
		}, action: "unknown", want: decisionAsk, check: func(t *testing.T, d chatDecision) {
			if d.Question != "Which format?" || d.Reason != askOptions || d.Key != "aspect_ratio" || strings.Join(d.Assumptions, ",") != "one,two,three" {
				t.Fatalf("%+v", d)
			}
			labels := []string{}
			for _, o := range d.Options {
				labels = append(labels, o.Label+"="+o.Value)
			}
			if strings.Join(labels, ",") != "Vertical=9:16 please,Wide=Wide,Square=Square,Cinema=Cinema" {
				t.Fatalf("options %v", labels)
			}
		}},
		{name: "bad key and reason are dropped", raw: &agents.ChatDecision{Mode: "ask", Question: "Which?", Reason: "because", Key: "Bad Key!"}, action: "unknown", want: decisionAsk, check: func(t *testing.T, d chatDecision) {
			if d.Reason != "" || d.Key != "" {
				t.Fatalf("%+v", d)
			}
		}},
		{name: "a long assumption is cut", raw: &agents.ChatDecision{Mode: "act", Assumptions: []string{long}}, action: "render_scene", want: decisionAct, check: func(t *testing.T, d chatDecision) {
			if n := len([]rune(d.Assumptions[0])); n != maxAssumptionChars {
				t.Fatalf("assumption is %d runes", n)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := normalizeChatDecision(tc.raw, tc.action)
			if d.Mode != tc.want || d.Assumptions == nil {
				t.Fatalf("mode %q, want %q: %+v", d.Mode, tc.want, d)
			}
			if tc.check != nil {
				tc.check(t, d)
			}
		})
	}
}

func TestFullAutoOnlyAsksWhatItMay(t *testing.T) {
	ask := func(reason string, options ...string) chatDecision {
		d := chatDecision{Mode: decisionAsk, Question: "Which music?", Reason: reason, Assumptions: []string{}}
		for _, o := range options {
			d.Options = append(d.Options, chatDecisionOption{Label: o, Value: o})
		}
		return d
	}
	for _, tc := range []struct {
		name         string
		d            chatDecision
		action       string
		inputMissing bool
		want         string
	}{
		{"plan confirmation", ask(askConfirmPlan), "generate_script", false, decisionAsk},
		{"plan confirmation for another step", ask(askConfirmPlan), "render_scenes", false, decisionAct},
		{"destructive step", ask(askDestructive), "delete_scene", false, decisionAsk},
		{"destructive claim for a safe step", ask(askDestructive), "generate_shots", false, decisionAct},
		{"missing input with nothing to run", ask(askMissingInput), "unknown", false, decisionAsk},
		{"missing input the project shows", ask(askMissingInput), "render_scenes", true, decisionAsk},
		{"missing input the project does not show", ask(askMissingInput), "render_scenes", false, decisionAct},
		{"options", ask(askOptions, "Piano", "Strings"), "finish_video", false, decisionAct},
		{"expensive", ask(askExpensive), "render_scenes", false, decisionAct},
		{"no reason", ask(""), "generate_character_references", false, decisionAct},
		{"no action leaves the question", ask(askOptions), "unknown", false, decisionAsk},
		{"not a question", chatDecision{Mode: decisionAct, Assumptions: []string{}}, "render_scenes", false, decisionAct},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := applyFullAutoAsk(tc.d, tc.action, tc.inputMissing)
			if got.Mode != tc.want {
				t.Fatalf("mode %q, want %q: %+v", got.Mode, tc.want, got)
			}
			overruled := tc.d.Mode == decisionAsk && tc.want == decisionAct
			if overruled != (got.Overruled == "full_auto") || overruled && (got.Question != "" || got.Options != nil || len(got.Assumptions) != 1) {
				t.Fatalf("%+v", got)
			}
		})
	}
	got := applyFullAutoAsk(ask(askOptions, "Piano", "Strings"), "finish_video", false)
	if !strings.Contains(got.Assumptions[0], "“Piano”") {
		t.Fatalf("assumption %q does not name the default", got.Assumptions[0])
	}
}

// Through /chat: Full auto overrules a question, Manual keeps it, and the plan
// confirmation is still asked in Full auto.
func TestChatEnforcesTheAskRuleServerSide(t *testing.T) {
	question := `{"mode":"ask","question":"Which scenes first?","reason":"options","options":[{"label":"All of them","value":"make all"},{"label":"Just the first","value":"first only"}]}`
	answer := `{"reply":"I'll make the videos.","action":"render_scenes","decision":` + question + `}`
	s := routerServer(t, nil)
	plannerSays(s, &answer)
	storyScenes(t, s, "Opening", "Ending")
	decide := func(body string) chatDecision {
		t.Helper()
		w := call(t, s, "POST", "/chat", body)
		var out struct {
			Intent struct {
				Action   string       `json:"action"`
				Decision chatDecision `json:"decision"`
			} `json:"intent"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatal(w.Code, w.Body.String())
		}
		return out.Intent.Decision
	}
	if d := decide(`{"message":"make the videos","auto":true,"full":true}`); d.Mode != decisionAct || d.Overruled != "full_auto" || !strings.Contains(strings.Join(d.Assumptions, " "), "All of them") {
		t.Fatalf("full auto: %+v", d)
	}
	if d := decide(`{"message":"make the videos"}`); d.Mode != decisionAsk || len(d.Options) != 2 || d.Overruled != "" {
		t.Fatalf("manual: %+v", d)
	}
	if d := decide(`{"message":"make the videos","auto":true}`); d.Mode != decisionAsk {
		t.Fatalf("auto: %+v", d)
	}
	answer = `{"reply":"Here is the plan.","action":"generate_script","creation_brief":{"idea":"a surfing dog"},"decision":{"mode":"ask","question":"Shall I make it?","reason":"confirm_plan","assumptions":["9:16 for TikTok"]}}`
	if d := decide(`{"message":"a surfing dog video","auto":true,"full":true}`); d.Mode != decisionAsk || d.Reason != askConfirmPlan || d.Assumptions[0] != "9:16 for TikTok" {
		t.Fatalf("plan confirmation: %+v", d)
	}
	// An older planner reply with no decision still works.
	answer = `{"reply":"ok","action":"none"}`
	if d := decide(`{"message":"hi"}`); d.Mode != decisionInform {
		t.Fatalf("no decision: %+v", d)
	}
}
