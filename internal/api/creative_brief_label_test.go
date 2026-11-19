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

func TestBriefDefaultsFollowPlatformAndClipLength(t *testing.T) {
	for _, tc := range []struct {
		platform, wantAspect string
	}{{"YouTube Shorts", "9:16"}, {"TikTok", "9:16"}, {"Instagram Reels", "9:16"}, {"YouTube", "16:9"}} {
		b := agents.CreativeBrief{Platform: tc.platform}
		fillCreativeBriefDefaults(&b, true)
		if b.AspectRatio != tc.wantAspect || b.PresetID != "project-style" || b.Resolution != "720p" {
			t.Fatalf("%s: %+v", tc.platform, b)
		}
	}
	// 20 seconds in one scene is longer than a clip can be: plan two scenes.
	b := agents.CreativeBrief{TargetDuration: 20, SceneCount: 1, AspectRatio: "9:16"}
	fillCreativeBriefDefaults(&b, false)
	if b.SceneCount != 2 || b.Platform != "TikTok" || b.PresetID != "" {
		t.Fatalf("%+v", b)
	}
	// An explicit choice that already fits is kept.
	b = agents.CreativeBrief{TargetDuration: 30, SceneCount: 5, Platform: "YouTube", AspectRatio: "1:1"}
	fillCreativeBriefDefaults(&b, false)
	if b.SceneCount != 5 || b.AspectRatio != "1:1" {
		t.Fatalf("%+v", b)
	}
}
