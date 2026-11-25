package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeA2UIKeepsAllowedComponents(t *testing.T) {
	raw := `[
		{"id":"root","component":"Column","children":["title","pick","cmp","sep"]},
		{"id":"title","component":"Text","text":"Which look?","variant":"h3","onClick":"evil()"},
		{"id":"pick","component":"ChoicePicker","options":[{"label":"Warm","value":"Use warm"},{"label":"Cold","value":"Use cold"}],"value":{"@path":"/secret"}},
		{"id":"cmp","component":"Row","children":["c1","c2"]},
		{"id":"c1","component":"Card","child":"i1"},
		{"id":"i1","component":"Image","url":"/media/a.png","description":"Warm"},
		{"id":"c2","component":"Button","child":"bt","variant":"primary","action":{"event":{"name":"render_scenes","context":{"reply":"Use cold","price":"0"}}}},
		{"id":"bt","component":"Text","text":"Cold"},
		{"id":"sep","component":"Divider"}
	]`
	got := sanitizeA2UI(json.RawMessage(raw))
	if len(got) != 9 || got[0]["id"] != a2uiUIRoot {
		t.Fatalf("%v", got)
	}
	byID := map[string]map[string]any{}
	for _, c := range got {
		byID[c["id"].(string)] = c
	}
	if _, has := byID["ui-title"]["onClick"]; has {
		t.Fatal("an unknown property survived")
	}
	if mustJSON(byID["ui-c2"]["action"]) != `{"event":{"context":{"reply":"Use cold"},"name":"videoflow.reply"}}` {
		t.Fatalf("button action %v", byID["ui-c2"]["action"])
	}
	if mustJSON(byID["ui-pick"]["value"]) != `{"@path":"/choice"}` || byID["ui-pick"]["variant"] != "mutuallyExclusive" {
		t.Fatalf("picker %v", byID["ui-pick"])
	}
	if mustJSON(byID["ui-cmp"]["children"]) != `["ui-c1","ui-c2"]` {
		t.Fatalf("children %v", byID["ui-cmp"])
	}
}

func TestSanitizeA2UIDropsAnythingInvalid(t *testing.T) {
	long := strings.Repeat("x", a2uiMaxText+1)
	many := []string{}
	children := []string{}
	for i := 0; i <= a2uiMaxComponents; i++ {
		id := "t" + string(rune('a'+i%26)) + strings.Repeat("z", i/26)
		children = append(children, `"`+id+`"`)
		many = append(many, `{"id":"`+id+`","component":"Text","text":"x"}`)
	}
	cases := map[string]string{
		"empty":             `[]`,
		"not a list":        `{"id":"root","component":"Text","text":"x"}`,
		"no root":           `[{"id":"a","component":"Text","text":"x"}]`,
		"unknown kind":      `[{"id":"root","component":"TextField","label":"Card number"}]`,
		"custom component":  `[{"id":"root","component":"VideoFlowAction","intent":{"action":"render_scenes"}}]`,
		"bound text":        `[{"id":"root","component":"Text","text":{"@path":"/state"}}]`,
		"function call":     `[{"id":"root","component":"Text","text":{"@call":"fetch"}}]`,
		"long text":         `[{"id":"root","component":"Text","text":"` + long + `"}]`,
		"bad variant":       `[{"id":"root","component":"Text","text":"x","variant":"blink"}]`,
		"script image":      `[{"id":"root","component":"Image","url":"javascript:alert(1)"}]`,
		"protocol-relative": `[{"id":"root","component":"Image","url":"//evil.example/a.png"}]`,
		"button no reply":   `[{"id":"root","component":"Button","child":"t","action":{"event":{"name":"generate_video"}}},{"id":"t","component":"Text","text":"Go"}]`,
		"missing child":     `[{"id":"root","component":"Column","children":["nope"]}]`,
		"cycle":             `[{"id":"root","component":"Column","children":["a"]},{"id":"a","component":"Card","child":"root"}]`,
		"shared child":      `[{"id":"root","component":"Row","children":["a","a"]},{"id":"a","component":"Text","text":"x"}]`,
		"unreachable":       `[{"id":"root","component":"Text","text":"x"},{"id":"a","component":"Text","text":"y"}]`,
		"duplicate id":      `[{"id":"root","component":"Text","text":"x"},{"id":"root","component":"Text","text":"y"}]`,
		"bad id":            `[{"id":"root","component":"Column","children":["a b"]},{"id":"a b","component":"Text","text":"y"}]`,
		"one option":        `[{"id":"root","component":"ChoicePicker","options":[{"label":"A","value":"a"}]}]`,
		"too many":          `[{"id":"root","component":"Column","children":[` + strings.Join(children[:a2uiMaxChildren], ",") + `]},` + strings.Join(many, ",") + `]`,
	}
	for name, raw := range cases {
		if got := sanitizeA2UI(json.RawMessage(raw)); got != nil {
			t.Errorf("%s: kept %v", name, got)
		}
	}
	if sanitizeA2UI(nil) != nil || sanitizeA2UI(json.RawMessage("null")) != nil {
		t.Fatal("no ui is nil")
	}
}

// An invalid ui is dropped; the reply and the card are still shown.
func TestChatDropsInvalidUIButKeepsTheReply(t *testing.T) {
	s := routerServer(t, nil)
	answer := `{"reply":"Here is the plan.","action":"unknown","decision":{"mode":"inform"},"ui":[{"id":"root","component":"Button","child":"x","action":{"event":{"name":"render_scenes"}}}]}`
	plannerSays(s, &answer)
	result := call(t, s, "POST", "/chat", `{"message":"A story"}`)
	var body map[string]any
	_ = json.Unmarshal(result.Body.Bytes(), &body)
	intent := body["intent"].(map[string]any)
	if result.Code != 200 || intent["reply"] != "Here is the plan." {
		t.Fatalf("%d %v", result.Code, body)
	}
	if _, has := intent["ui"]; has {
		t.Fatalf("invalid ui kept: %v", intent["ui"])
	}
}
