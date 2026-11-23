package api

import (
	"fmt"
	"strings"
	"testing"

	"videoflow-go/internal/agents"
	"videoflow-go/internal/db"
)

// The assistant evaluation, deterministic half: how the planner's answer is
// post-processed. One table over assistant mode × the planner's decision × the decision
// log × the project's stage × the user's words, through chatTurnDecision, the same
// function every /chat turn ends with. Each row states its expected outcome by hand.

// Project states a turn can see.
const (
	stageNormal   = "normal"   // scenes ready to render, nothing blocked
	stageBlockedS = "blocked"  // a scene waits on the video service
	stageNoKey    = "no_key"   // the video service refused its key
	stageNoScenes = "no_story" // nothing written yet
)

func matrixTurn(mode, stage, message string, log []db.ProjectDecision) *chatTurn {
	ready := []stageScene{{ID: "s1", Title: "Opening", Progress: progressAt("ready_to_render", "none"), VideoSeconds: 8}, {ID: "s2", Title: "Ending", Progress: progressAt("ready_to_render", "none"), VideoSeconds: 8}}
	in := stageInputs{BriefDecided: true, Scenes: ready}
	service := map[string]any{"service": "fal.ai"}
	switch stage {
	case stageBlockedS:
		in.Scenes[0].Progress = progressAt("needs_attention", "waiting_on_service")
	case stageNoKey:
		service["problem"] = "fal.ai refused its key"
	case stageNoScenes:
		in = stageInputs{}
	}
	turn := &chatTurn{stages: computeStages(in), videoService: service, decisions: log}
	turn.body.Message = message
	turn.body.Auto = mode != "manual"
	turn.body.Full = mode == "full"
	return turn
}

// Planner decisions.
func plannerAsk(reason, key string, options ...string) *agents.ChatDecision {
	d := &agents.ChatDecision{Mode: "ask", Question: "Which one?", Reason: reason, Key: key}
	for _, o := range options {
		d.Options = append(d.Options, agents.DecisionOption{Label: o})
	}
	return d
}

var decidedFormat = []db.ProjectDecision{{Key: "aspect_ratio", Value: "9:16", Source: db.DecisionByUser}}

func TestChatDecisionMatrix(t *testing.T) {
	type row struct {
		mode, stage, message string
		log                  []db.ProjectDecision
		decision             *agents.ChatDecision
		action               string
		// want is the mode the user sees; overruled is why a question became a decision.
		want, overruled string
		// assumption is a phrase one assumption must contain.
		assumption string
	}
	rows := []row{
		// Full auto asks only the plan confirmation, a destructive step or a missing input.
		{mode: "full", stage: stageNormal, message: "make the videos", decision: plannerAsk(askOptions, "", "All of them", "Just one"), action: "render_scenes", want: decisionAct, overruled: "full_auto", assumption: "“All of them”"},
		{mode: "full", stage: stageNormal, message: "make the videos", decision: plannerAsk(askExpensive, ""), action: "render_scenes", want: decisionAct, overruled: "full_auto", assumption: "Which one?"},
		{mode: "full", stage: stageNormal, message: "pictures", decision: plannerAsk("", ""), action: "generate_character_references", want: decisionAct, overruled: "full_auto"},
		{mode: "full", stage: stageNoScenes, message: "a surfing dog", decision: plannerAsk(askConfirmPlan, ""), action: "generate_script", want: decisionAsk},
		{mode: "full", stage: stageNormal, message: "make the videos", decision: plannerAsk(askConfirmPlan, ""), action: "render_scenes", want: decisionAct, overruled: "full_auto"},
		{mode: "full", stage: stageNormal, message: "remove the ending", decision: plannerAsk(askDestructive, ""), action: "delete_scene", want: decisionAsk},
		{mode: "full", stage: stageNormal, message: "plan shots", decision: plannerAsk(askDestructive, ""), action: "generate_shots", want: decisionAct, overruled: "full_auto"},
		{mode: "full", stage: stageNoScenes, message: "make a video", decision: plannerAsk(askMissingInput, "", "A dog", "A cat"), action: "unknown", want: decisionAsk},
		{mode: "full", stage: stageBlockedS, message: "make the videos", decision: plannerAsk(askMissingInput, ""), action: "render_scenes", want: decisionAsk},
		{mode: "full", stage: stageNoKey, message: "make the videos", decision: plannerAsk(askMissingInput, ""), action: "render_scenes", want: decisionAsk},
		{mode: "full", stage: stageNormal, message: "make the videos", decision: plannerAsk(askMissingInput, ""), action: "render_scenes", want: decisionAct, overruled: "full_auto"},
		{mode: "full", stage: stageNormal, message: "what music?", decision: plannerAsk(askOptions, "", "Piano"), action: "unknown", want: decisionAsk},
		// Manual and auto keep a valid question.
		{mode: "manual", stage: stageNormal, message: "make the videos", decision: plannerAsk(askOptions, "", "All of them", "Just one"), action: "render_scenes", want: decisionAsk},
		{mode: "auto", stage: stageNormal, message: "make the videos", decision: plannerAsk(askExpensive, ""), action: "render_scenes", want: decisionAsk},
		{mode: "auto", stage: stageNormal, message: "pictures", decision: plannerAsk("", ""), action: "generate_character_references", want: decisionAsk},
		// The decision log settles a question, in every mode, unless the user changes it.
		{mode: "manual", stage: stageNormal, message: "make the videos", log: decidedFormat, decision: plannerAsk(askOptions, "aspect_ratio", "Wide"), action: "render_scenes", want: decisionAct, overruled: "already_decided", assumption: "aspect_ratio = 9:16"},
		{mode: "auto", stage: stageNormal, message: "make the videos", log: decidedFormat, decision: plannerAsk(askOptions, "aspect_ratio", "Wide"), action: "render_scenes", want: decisionAct, overruled: "already_decided"},
		{mode: "full", stage: stageNormal, message: "make the videos", log: decidedFormat, decision: plannerAsk(askOptions, "aspect_ratio", "Wide"), action: "render_scenes", want: decisionAct, overruled: "already_decided"},
		{mode: "manual", stage: stageNormal, message: "let's change the format", log: decidedFormat, decision: plannerAsk(askOptions, "aspect_ratio", "Wide"), action: "render_scenes", want: decisionAsk},
		{mode: "manual", stage: stageNormal, message: "switch to wide", log: decidedFormat, decision: plannerAsk(askOptions, "aspect_ratio"), action: "render_scenes", want: decisionAsk},
		{mode: "manual", stage: stageNormal, message: "hazlo en otro formato", log: decidedFormat, decision: plannerAsk(askOptions, "aspect_ratio"), action: "render_scenes", want: decisionAsk},
		{mode: "manual", stage: stageNormal, message: "plutôt en 16:9", log: decidedFormat, decision: plannerAsk(askOptions, "aspect_ratio"), action: "render_scenes", want: decisionAsk},
		{mode: "manual", stage: stageNormal, message: "换成横屏", log: decidedFormat, decision: plannerAsk(askOptions, "aspect_ratio"), action: "render_scenes", want: decisionAsk},
		// Known limitation: change words of other languages are not recognised, so the
		// earlier choice is kept. Extend changeWords and flip these rows to fix it.
		{mode: "manual", stage: stageNormal, message: "ändere das Format", log: decidedFormat, decision: plannerAsk(askOptions, "aspect_ratio"), action: "render_scenes", want: decisionAct, overruled: "already_decided"},
		{mode: "manual", stage: stageNormal, message: "形式を変えて", log: decidedFormat, decision: plannerAsk(askOptions, "aspect_ratio"), action: "render_scenes", want: decisionAct, overruled: "already_decided"},
		// The log does not settle other keys, a question with no step, or a safety question.
		{mode: "manual", stage: stageNormal, message: "add music", log: decidedFormat, decision: plannerAsk(askOptions, "music", "Piano"), action: "render_scenes", want: decisionAsk},
		{mode: "manual", stage: stageNormal, message: "which format?", log: decidedFormat, decision: plannerAsk(askOptions, "aspect_ratio"), action: "unknown", want: decisionAsk},
		{mode: "full", stage: stageNoScenes, message: "a surfing dog", log: decidedFormat, decision: plannerAsk(askConfirmPlan, "aspect_ratio"), action: "generate_script", want: decisionAsk},
		{mode: "manual", stage: stageNormal, message: "remove the ending", log: decidedFormat, decision: plannerAsk(askDestructive, "aspect_ratio"), action: "delete_scene", want: decisionAsk},
		// Invalid or missing decisions default safely.
		{mode: "full", stage: stageNormal, message: "make the videos", action: "render_scenes", want: decisionAct},
		{mode: "manual", stage: stageNormal, message: "hi", action: "unknown", want: decisionInform},
		{mode: "manual", stage: stageNormal, message: "make the videos", decision: &agents.ChatDecision{Mode: "perhaps"}, action: "render_scenes", want: decisionInform},
		{mode: "manual", stage: stageNormal, message: "hi", decision: &agents.ChatDecision{Mode: "act"}, action: "unknown", want: decisionInform},
		{mode: "full", stage: stageNormal, message: "make the videos", decision: &agents.ChatDecision{Mode: "ask", Reason: askOptions}, action: "render_scenes", want: decisionInform},
		{mode: "manual", stage: stageNormal, message: "make the videos", decision: &agents.ChatDecision{Mode: " ASK ", Question: "Which one?", Reason: "whim"}, action: "render_scenes", want: decisionAsk},
		{mode: "full", stage: stageNormal, message: "make the videos", decision: &agents.ChatDecision{Mode: " ASK ", Question: "Which one?", Reason: "whim"}, action: "render_scenes", want: decisionAct, overruled: "full_auto"},
	}
	for i, r := range rows {
		t.Run(fmt.Sprintf("%02d %s %s %s %q", i, r.mode, r.stage, r.action, r.message), func(t *testing.T) {
			turn := matrixTurn(r.mode, r.stage, r.message, r.log)
			got := chatTurnDecision(turn, &agents.ChatIntentResponse{Action: r.action, Decision: r.decision}, r.action)
			if got.Mode != r.want || got.Overruled != r.overruled {
				t.Fatalf("mode %q overruled %q, want %q %q: %+v", got.Mode, got.Overruled, r.want, r.overruled, got)
			}
			checkDecisionShape(t, got)
			if r.overruled != "" && (len(got.Assumptions) == 0 || !strings.Contains(got.Assumptions[len(got.Assumptions)-1], r.assumption)) {
				t.Fatalf("an overruled question must name what was assumed (%q): %+v", r.assumption, got.Assumptions)
			}
		})
	}
}

// checkDecisionShape holds for every decision the user receives.
func checkDecisionShape(t *testing.T, d chatDecision) {
	t.Helper()
	if d.Assumptions == nil || len(d.Assumptions) > maxAssumptions {
		t.Fatalf("assumptions %v", d.Assumptions)
	}
	for _, a := range d.Assumptions {
		if a == "" || len([]rune(a)) > maxAssumptionChars {
			t.Fatalf("assumption %q", a)
		}
	}
	if d.Mode == decisionAsk {
		if d.Question == "" || len([]rune(d.Question)) > maxQuestionChars || len(d.Options) > maxDecisionOptions {
			t.Fatalf("question %+v", d)
		}
		for _, o := range d.Options {
			if o.Label == "" || o.Value == "" || len([]rune(o.Label)) > maxOptionLabel || len([]rune(o.Value)) > maxOptionValue {
				t.Fatalf("option %+v", o)
			}
		}
		return
	}
	if d.Question != "" || d.Options != nil || d.Reason != "" {
		t.Fatalf("a decision that does not ask carries a question: %+v", d)
	}
}

// Caps hold through every overrule: at most three assumptions, the overrule's own last.
func TestChatDecisionCapsSurviveOverrules(t *testing.T) {
	long := strings.Repeat("é", 2*maxAssumptionChars)
	many := []string{"one", "two", "three", "four", long}
	options := []agents.DecisionOption{}
	for i := 0; i < 9; i++ {
		options = append(options, agents.DecisionOption{Label: fmt.Sprintf("Option %d %s", i, long), Value: long})
	}
	for _, tc := range []struct {
		mode, message string
		log           []db.ProjectDecision
		overruled     string
	}{
		{"manual", "make the videos", nil, ""},
		{"full", "make the videos", nil, "full_auto"},
		{"manual", "make the videos", decidedFormat, "already_decided"},
	} {
		raw := &agents.ChatDecision{Mode: "ask", Question: long, Reason: askOptions, Key: "aspect_ratio", Assumptions: many, Options: options}
		got := chatTurnDecision(matrixTurn(tc.mode, stageNormal, tc.message, tc.log), &agents.ChatIntentResponse{Decision: raw}, "render_scenes")
		if got.Overruled != tc.overruled {
			t.Fatalf("%s: %+v", tc.mode, got)
		}
		checkDecisionShape(t, got)
		if len(got.Assumptions) != maxAssumptions || got.Assumptions[0] != "one" {
			t.Fatalf("%s: assumptions %v", tc.mode, got.Assumptions)
		}
		if tc.overruled == "" && (len(got.Options) != maxDecisionOptions || !strings.HasSuffix(got.Question, "…")) {
			t.Fatalf("%s: question not capped: %d options, %q", tc.mode, len(got.Options), got.Question)
		}
	}
}

// What the planner may remember: known keys or prefixes with a valid id, never a
// rejection or a quick-reply choice, which only the user records.
func TestRememberKeyMatrix(t *testing.T) {
	for key, want := range map[string]bool{
		"language": true, "style_id": true, "aspect_ratio": true, "target_duration": true, "platform": true, "music": true,
		"conversation_mode": true, "tone": true, "audience": true, "resolution": true, "notes": true, "reviewed": true,
		"voice:char_1a2b": true, "character:c1": true, "scene:scene_9f": true, "notes:pacing": true,
		"voice:": false, "voice": false, "Language": false, " language": false, "language:en": false,
		"rejected:render_scene:s1": false, "choice:which_format": false, "api_key": false, "secret": false,
		"voice:a b": false, strings.Repeat("a", maxDecisionKeyChars+1): false, "": false,
	} {
		if got := assistantMayRemember(key); got != want {
			t.Errorf("assistantMayRemember(%q) = %v, want %v", key, got, want)
		}
	}
}
