package presets

import (
	"strings"
	"testing"
)

func TestStorytellingIndependentOfVisualStyle(t *testing.T) {
	if len(StorytellingPresets()) < 4 {
		t.Fatal("missing storytelling choices")
	}
	for _, p := range StorytellingPresets() {
		if p.ID == "" || p.Name == "" || p.Description == "" {
			t.Fatal("incomplete catalog")
		}
		guidance, err := StorytellingDirection(p.ID)
		if err != nil || !strings.Contains(guidance, p.Name) || !strings.Contains(guidance, "existing") {
			t.Fatalf("missing production guidance: %s", p.ID)
		}
	}
	if direction, err := StorytellingDirection(""); err != nil || direction != "" {
		t.Fatal("legacy brief changed")
	}
	if _, err := StorytellingDirection("invented"); err == nil {
		t.Fatal("accepted unknown preset")
	}
}
