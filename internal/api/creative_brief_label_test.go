package api

import (
	"strings"
	"testing"

	"videoflow-go/internal/agents"
)

func TestCreativeBriefQuestionNamesTheLanguage(t *testing.T) {
	for in, want := range map[string]string{"en": "English", "zh-CN": "Chinese", "pt_BR": "Portuguese", "Klingon": "Klingon"} {
		if got := languageLabel(in); got != want {
			t.Fatalf("languageLabel(%q) = %q, want %q", in, got, want)
		}
	}
	q := creativeBriefQuestion(agents.CreativeBrief{Idea: "space", Language: "en", AspectRatio: "16:9"})
	if !strings.Contains(q, "English") || strings.Contains(q, " en,") {
		t.Fatalf("question should name the language: %q", q)
	}
}
