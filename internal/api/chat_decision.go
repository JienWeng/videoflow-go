package api

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"videoflow-go/internal/agents"
)

// Ask or decide. Each chat reply carries a decision: "act" (the assistant goes ahead
// with its action), "ask" (one question the user must answer first, with quick-reply
// options) or "inform" (an answer with no step to take), plus the assumptions it made
// instead of asking.
//
// The rule, here and in chat_prompt.md: ask only when
//   (a) a required input is missing and has no sensible default,
//   (b) the step is expensive or cannot be undone beyond what the mode allows, or
//   (c) there are materially different options the decision log does not settle.
// Otherwise decide, name the assumption in one line, and proceed.
//
// The server enforces what it can check. In Full auto the only questions allowed are
// the one plan confirmation, a destructive step, a missing required input, and whether
// to re-render a clip the self-check flagged (video credits, see selfcheck_policy.go);
// any other question with an action to take becomes "act", noting the assumption.

// Decision modes.
const (
	decisionAct    = "act"
	decisionAsk    = "ask"
	decisionInform = "inform"
)

// Reasons the planner may give for asking.
const (
	askMissingInput = "missing_input"
	askExpensive    = "expensive"
	askOptions      = "options"
	askConfirmPlan  = "confirm_plan"
	askDestructive  = "destructive"
)

// Limits on what a reply's decision may carry.
const (
	maxAssumptions     = 3
	maxAssumptionChars = 200
	maxQuestionChars   = 300
	maxDecisionOptions = 4
	maxOptionLabel     = 60
	maxOptionValue     = 300
)

// chatDecisionOption is one quick reply: Label is shown on the chip, Value is sent as
// the user's message when it is picked.
type chatDecisionOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// chatDecision is the validated decision sent with every chat reply (intent.decision).
type chatDecision struct {
	Mode        string               `json:"mode"`
	Assumptions []string             `json:"assumptions"`
	Question    string               `json:"question,omitempty"`
	Options     []chatDecisionOption `json:"options,omitempty"`
	Reason      string               `json:"reason,omitempty"`
	// Key is the decision-log key the answer settles (see decisions.go).
	Key string `json:"key,omitempty"`
	// Overruled says the server turned the planner's question into a decision:
	// "full_auto" (asking is not allowed there) or "already_decided" (the decision log
	// settles it).
	Overruled string `json:"overruled,omitempty"`
}

// clip trims s and cuts it to at most n characters, on a rune boundary.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:n-1])) + "…"
}

// normalizeChatDecision validates the planner's decision for a turn whose resolved
// action is action ("unknown" when there is none). A missing decision is "act" when
// there is an action and "inform" otherwise; an unknown mode, or a question without
// text, is "inform".
func normalizeChatDecision(raw *agents.ChatDecision, action string) chatDecision {
	d := chatDecision{Assumptions: []string{}}
	if raw == nil {
		d.Mode = decisionInform
		if action != "unknown" {
			d.Mode = decisionAct
		}
		return d
	}
	for _, a := range raw.Assumptions {
		if a = clip(a, maxAssumptionChars); a != "" && len(d.Assumptions) < maxAssumptions {
			d.Assumptions = append(d.Assumptions, a)
		}
	}
	switch mode := strings.ToLower(strings.TrimSpace(raw.Mode)); mode {
	case decisionAct, decisionAsk, decisionInform:
		d.Mode = mode
	default:
		d.Mode = decisionInform
	}
	if d.Mode == decisionAct && action == "unknown" {
		d.Mode = decisionInform
	}
	if d.Mode != decisionAsk {
		return d
	}
	d.Question = clip(raw.Question, maxQuestionChars)
	if d.Question == "" {
		d.Mode = decisionInform
		return d
	}
	seen := map[string]bool{}
	for _, o := range raw.Options {
		label, value := clip(o.Label, maxOptionLabel), clip(o.Value, maxOptionValue)
		if value == "" {
			value = label
		}
		if label == "" || seen[strings.ToLower(label)] || len(d.Options) == maxDecisionOptions {
			continue
		}
		seen[strings.ToLower(label)] = true
		d.Options = append(d.Options, chatDecisionOption{Label: label, Value: value})
	}
	switch reason := strings.TrimSpace(raw.Reason); reason {
	case askMissingInput, askExpensive, askOptions, askConfirmPlan, askDestructive:
		d.Reason = reason
	}
	if key := strings.TrimSpace(raw.Key); validDecisionKey(key) {
		d.Key = key
	}
	return d
}

// destructiveChatAction says whether an action deletes saved work.
func destructiveChatAction(action string) bool {
	return action == "delete_scene"
}

// fullAutoAskAllowed says whether Full auto may stop for this question: the plan
// confirmation, a destructive step, a required input that is missing (nothing can
// run, a stage is blocked, or the video service needs fixing), or re-rendering a clip
// the self-check flagged (clipFlagged).
func fullAutoAskAllowed(d chatDecision, action string, inputMissing, clipFlagged bool) bool {
	switch d.Reason {
	case askExpensive:
		return clipReviewAsk(d, action, clipFlagged)
	case askConfirmPlan:
		return action == "generate_script"
	case askDestructive:
		return destructiveChatAction(action)
	case askMissingInput:
		return action == "unknown" || inputMissing
	}
	return false
}

// applyFullAutoAsk enforces Full auto's rule: a question it does not allow, asked
// while there is an action to take, becomes "act" with the assumption noted. With no
// action the question stands, since there is nothing to go ahead with.
func applyFullAutoAsk(d chatDecision, action string, inputMissing, clipFlagged bool) chatDecision {
	if d.Mode != decisionAsk || action == "unknown" || fullAutoAskAllowed(d, action, inputMissing, clipFlagged) {
		return d
	}
	return overruleAsk(d, decisionAct, "full_auto", fullAutoAssumption(d))
}

func fullAutoAssumption(d chatDecision) string {
	if len(d.Options) > 0 {
		return clip("Went with “"+d.Options[0].Label+"” instead of asking (Full auto)", maxAssumptionChars)
	}
	return clip("Decided without asking (Full auto): "+d.Question, maxAssumptionChars)
}

// overruleAsk turns a question into mode, recording why and the assumption made.
func overruleAsk(d chatDecision, mode, why, assumption string) chatDecision {
	d.Mode, d.Overruled = mode, why
	d.Question, d.Options, d.Reason = "", nil, ""
	if len(d.Assumptions) == maxAssumptions {
		d.Assumptions = d.Assumptions[:maxAssumptions-1]
	}
	d.Assumptions = append(d.Assumptions, assumption)
	return d
}

// chatInputMissing says whether the project is waiting on something only the user can
// give: a stage is blocked, or the video service refused its key or has no balance.
func chatInputMissing(turn *chatTurn) bool {
	for _, st := range turn.stages.Stages {
		if st.Status == statusBlocked {
			return true
		}
	}
	problem, _ := turn.videoService["problem"].(string)
	return problem != ""
}

// decisionKeyPattern is a decision-log key: a lowercase name, optionally followed by
// ":"-separated parts such as a character id ("voice:char_1a2b3c4d") or what was
// rejected ("rejected:render_scene:scene_9f8e7d6c").
var decisionKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(:[A-Za-z0-9_.\-]+)*$`)

const maxDecisionKeyChars = 120

// validDecisionKey says whether key may name a decision-log entry.
func validDecisionKey(key string) bool {
	return len(key) <= maxDecisionKeyChars && decisionKeyPattern.MatchString(key)
}
