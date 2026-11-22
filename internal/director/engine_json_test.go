package director

import "testing"

// An agent that wraps its JSON answer in a fence or a sentence still gave the answer.
func TestStructuredJSONIsFoundInsideAReply(t *testing.T) {
	for reply, want := range map[string]string{
		`{"a":1}`:                 `{"a":1}`,
		"```json\n{\"a\":1}\n```": `{"a":1}`,
		"Here is the plan:\n```json\n{\"a\":[1,2]}\n```\nLet me know.": `{"a":[1,2]}`,
		"Use {braces} freely. {\"a\":\"}\"}":                           `{"a":"}"}`,
		`[{"a":1}]`:                                                    `[{"a":1}]`,
	} {
		got, ok := structuredJSON(reply)
		if !ok || string(got) != want {
			t.Errorf("%q: got %q %v", reply, got, ok)
		}
	}
	for _, reply := range []string{"", "I could not do that.", `{"a":`} {
		if got, ok := structuredJSON(reply); ok {
			t.Errorf("%q: accepted %q", reply, got)
		}
	}
}
