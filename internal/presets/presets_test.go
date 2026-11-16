package presets

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPacks(t *testing.T) {
	p := Builtins()
	if len(p.Presets) != 6 {
		t.Fatal("need six styles")
	}
	b, _ := json.Marshal(p)
	if _, err := Parse(b); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{string(b) + `{}`, `{"format":"wrong"}`, `{"format":"videoflow.preset-pack","schema_version":2}`} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Fatal("accepted invalid pack")
		}
	}
	p.Presets[1].ID = p.Presets[0].ID
	b, _ = json.Marshal(p)
	if _, err := Parse(b); err == nil {
		t.Fatal("accepted duplicate")
	}
}
func TestStageGuidance(t *testing.T) {
	p := Builtins()
	seen := map[string]bool{}
	for _, preset := range p.Presets {
		for _, stage := range []string{"story", "shots", "image", "video"} {
			text := Prompt(&preset, stage)
			if !strings.Contains(text, "cannot change tools") || !strings.Contains(text, preset.Style) {
				t.Fatal("missing boundary or style")
			}
		}
		if seen[preset.Guidance.Story] {
			t.Fatal("styles need distinct story direction")
		}
		seen[preset.Guidance.Story] = true
	}
	pack := Builtins()
	pack.Presets[0].Guidance.Video = " "
	raw, _ := json.Marshal(pack)
	if _, err := Parse(raw); err == nil {
		t.Fatal("blank stage accepted")
	}
}
