package api

import (
	"strings"
	"testing"

	"videoflow-go/internal/agents"
)

func TestCreativeBriefQuestionNamesTheLanguage(t *testing.T) {
	for in, want := range map[string]string{"en": "English", "zh-CN": "Mandarin", "pt_BR": "Portuguese", "Klingon": "Klingon"} {
		if got := languageLabel(in); got != want {
			t.Fatalf("languageLabel(%q) = %q, want %q", in, got, want)
		}
	}
	if got := mergeCreativeBrief(nil, &agents.CreativeBrief{Language: "en"}, "idea").Language; got != "English" {
		t.Fatalf("merged brief language = %q, want English so the form's menu shows it", got)
	}
	q := creativeBriefQuestion(agents.CreativeBrief{Idea: "space", Language: "en", AspectRatio: "16:9"})
	if !strings.Contains(q, "English") || strings.Contains(q, " en,") {
		t.Fatalf("question should name the language: %q", q)
	}
}
