package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestScriptAlignmentUsesExactApprovedWording(t *testing.T) {
	lines := []SpokenLine{{ID: "a", CharacterID: "c", Language: "en", Text: "Hello, world!"}, {ID: "b", CharacterID: "c", Language: "en", Text: "Don't drift."}}
	raw := json.RawMessage(`{"words":[{"text":"Hello,","start":0.2,"end":0.5},{"text":"world!","start":0.6,"end":1.1},{"text":"Don't","start":1.4,"end":1.8},{"text":"drift.","start":1.8,"end":2.3}]}`)
	cues, err := alignSpokenLines(lines, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 2 || cues[0].ID != "a" || cues[0].CharacterID != "c" || cues[0].Language != "en" || cues[0].Text != "Hello, world!" || cues[1].Text != "Don't drift." || cues[1].Start != 1.4 || cues[1].End != 2.3 {
		t.Fatalf("%+v", cues)
	}
	srt, err := exportScriptCaptions(cues, "srt")
	if err != nil || !strings.Contains(srt, "00:00:00,200 --> 00:00:01,100\nHello, world!") {
		t.Fatalf("%s %v", srt, err)
	}
	vtt, _ := exportScriptCaptions(cues, "vtt")
	if !strings.HasPrefix(vtt, "WEBVTT\n") {
		t.Fatal(vtt)
	}
}
func TestScriptAlignmentRejectsDriftAndInvalidTiming(t *testing.T) {
	for _, raw := range []string{`{"words":[{"text":"other","start":0,"end":1}]}`, `{"words":[{"text":"Hi","start":2,"end":1}]}`, `{"words":[]}`} {
		if _, err := alignSpokenLines([]SpokenLine{{ID: "line", Text: "Hi"}}, json.RawMessage(raw)); err == nil {
			t.Fatal("accepted drift/invalid alignment", raw)
		}
	}
}
