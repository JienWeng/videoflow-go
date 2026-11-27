package seedance

import (
	"regexp"
	"strings"
	"testing"
)

// The Gemini Omni guide sections are checked against Google's official prompt guide.
// These tests pin the source header and the rules taken from it, so a careless edit
// is caught; re-verify against the web page (docs/PROVIDERS.md, "Keeping the Omni
// guide current") when Google updates it.
func TestOmniGuidesFollowGoogle(t *testing.T) {
	checked := regexp.MustCompile(`checked \d{1,2} [A-Z][a-z]+ 20\d\d`)
	rules := map[string][]string{
		"speech-omni": {"Sound design:", "in quotes", "No dialogue.", "English is fully supported", "never two lines at the same moment"},
		"omni-visual": {"single continuous shot, no scene cuts", "no negative-prompt field", "[0-5s]", "exact words", "reference image", "3–15 s"},
	}
	for name, want := range rules {
		raw, err := guide.ReadFile("guide/" + name + ".md")
		if err != nil {
			t.Fatal(err)
		}
		head := strings.SplitN(string(raw), "\n", 2)[0]
		if !strings.HasPrefix(head, "<!-- Source: https://aistudio.google.com/docs/omni") || !checked.MatchString(head) {
			t.Errorf("%s: first line must cite the Omni guide URL and date checked, got %q", name, head)
		}
		body := section(name)
		if strings.Contains(body, "<!--") {
			t.Errorf("%s: source header leaks into the writer's playbook", name)
		}
		for _, rule := range want {
			if !strings.Contains(body, rule) {
				t.Errorf("%s: missing rule %q", name, rule)
			}
		}
	}
}

func TestOmniSingleShotAsksForNoCuts(t *testing.T) {
	one := Clip{Family: "gemini-omni-1.2-flash", Summary: "A quiet street", Shots: []Shot{{Action: "A man walks", Seconds: 5}}}
	if p := Compose(one); !strings.Contains(p, "In a single continuous shot, no scene cuts.") {
		t.Errorf("single-shot Omni prompt lacks the no-cuts line:\n%s", p)
	}
	two := one
	two.Shots = append(two.Shots, Shot{Action: "He stops", Seconds: 5})
	if p := Compose(two); strings.Contains(p, "no scene cuts") {
		t.Errorf("multi-shot Omni prompt must not forbid cuts:\n%s", p)
	}
	seed := one
	seed.Family = "seedance-2.0"
	if p := Compose(seed); strings.Contains(p, "no scene cuts") {
		t.Errorf("Seedance prompt got the Omni-only line:\n%s", p)
	}
	silent := one
	silent.Audio = Native
	if p := Compose(silent); !strings.Contains(p, "No dialogue. Sound design:") {
		t.Errorf("silent Omni clip should say \"No dialogue.\" as Google's guide does:\n%s", p)
	}
	if s := strings.Join(Sections(one), " "); !strings.Contains(s, "omni-visual") {
		t.Errorf("Omni clip does not read omni-visual: %s", s)
	}
	if s := strings.Join(Sections(seed), " "); strings.Contains(s, "omni-visual") {
		t.Errorf("Seedance clip reads omni-visual: %s", s)
	}
}
