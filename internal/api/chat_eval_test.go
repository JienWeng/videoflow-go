package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
)

// The assistant evaluation, deterministic half: what each chat turn is given. For one
// project per pipeline stage, built from saved state, a turn's prompt must carry the
// right stage map, the decision log, the assistant mode's rules, stay within a size
// budget and never carry a secret. The planner is a fake (plannerSays); nothing here
// starts a Codex or Claude CLI or calls a paid service: every provider request fails
// the test. The live, scored half is scripts/assistant_eval.py --suite stages.

// maxChatPromptBytes is the budget for the per-turn message (history, user message and
// project context). The standing instructions (chat_prompt.md) are budgeted apart.
const (
	maxChatPromptBytes = 24 << 10
	maxChatSystemBytes = 28 << 10
)

// Secrets the fixture server holds. None may reach the planner.
const (
	evalOpenRouterKey = "sk-or-v1-EVALSECRETopenrouter0123456789abcdef"
	evalFalKey        = "fal-EVALSECRET-0123456789abcdef"
	evalGeminiKey     = "AIzaEVALSECRETgemini0123456789abcdef"
	evalVaultKey      = "sk-EVALSECRET-vault-0123456789abcdef"
)

var secretShapes = regexp.MustCompile(`EVALSECRET|sk-[A-Za-z0-9_\-]{20,}|AIza[0-9A-Za-z_\-]{20,}|(?i)"(api_?key|secret|token|password)"\s*:\s*"[^"]+"`)

// evalServer is a project server holding keys for each service, so a leak would show.
// Reading the video model catalog (free, read-only; a scene's production plan asks for
// it) is answered with an empty list; any other outside request fails the test.
func evalServer(t *testing.T) *Server {
	t.Helper()
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && (r.URL.Path == "/models" || r.URL.Path == "/videos/models") {
			w.Write([]byte(`{"data":[]}`))
			return
		}
		t.Errorf("the chat turn called a service: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(500)
	})
	s.cfg.OpenRouterAPIKey, s.cfg.FalAPIKey, s.cfg.GeminiAPIKey = evalOpenRouterKey, evalFalKey, evalGeminiKey
	if err := s.database.SetProviderSecret("fal", evalVaultKey, nil); err != nil {
		t.Fatal(err)
	}
	return s
}

// evalFixture is one project state the assistant must read correctly.
type evalFixture struct {
	name string
	// build saves the project's state.
	build func(t *testing.T, s *Server, project string)
	// current and next are the stage map's current stage and next step.
	current, next string
	// lines are substrings of the compact per-stage lines, by stage id.
	lines map[string]string
	// summary is the stage map's one-line summary, in plain words.
	summary string
	// decided are decision-log keys the turn must carry.
	decided []string
}

func confirmBrief(s *Server, project string) {
	s.recordBriefDecisions(project, generationBrief{Language: "en", AspectRatio: "9:16", PresetID: "soft-3d-animation", Platform: "TikTok", TargetDuration: 20})
}

// renderedStory is a confirmed story whose two scenes each have a finished video.
func renderedStory(t *testing.T, s *Server, project string) {
	confirmBrief(s, project)
	characterWithSheet(t, s, project, "Pip")
	for _, scene := range storyScenes(t, s, "Opening", "Ending") {
		finishedVideo(t, s, scene)
	}
}

// savedEdit saves an edit (a composition manifest) made at the given time.
func savedEdit(t *testing.T, s *Server, project string, at time.Time) {
	t.Helper()
	dir := filepath.Join(s.compositionRoot(), project, "rev_eval")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifest, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(manifest, at, at); err != nil {
		t.Fatal(err)
	}
}

// newestClip is when the newest finished scene video was saved, as the stage map reads it.
func newestClip(t *testing.T, s *Server, project string) time.Time {
	t.Helper()
	in, err := s.stageInputs(t.Context(), project, projectProgress{})
	if err != nil || in.LastClipAt.IsZero() {
		t.Fatalf("no finished clip: %v", err)
	}
	return in.LastClipAt
}

var evalFixtures = []evalFixture{
	{name: "empty project", build: func(*testing.T, *Server, string) {}, current: stageBrief, next: "generate_script", summary: "Creative brief: ready — no story idea or creative brief yet. Next: Plan the story.",
		lines: map[string]string{stageBrief: "brief: ready — no story idea or creative brief yet", stageScript: "script: not_started"}},
	{name: "brief only", build: func(_ *testing.T, s *Server, project string) { confirmBrief(s, project) }, current: stageScript, next: "generate_script",
		summary: "Story & scenes: ready — no scenes written yet. Next: Write the story.",
		lines:   map[string]string{stageBrief: "brief: done", stageCharacters: "characters: done", stageScript: "script: ready — no scenes written yet"},
		decided: []string{"language", "aspect_ratio", "style_id", "platform", "target_duration"}},
	{name: "characters without sheets", build: func(t *testing.T, s *Server, project string) {
		confirmBrief(s, project)
		for _, name := range []string{"Pip", "Mama"} {
			if err := s.database.CreateCharacter(&models.Character{ProjectID: &project, Name: name}); err != nil {
				t.Fatal(err)
			}
		}
	}, current: stageCharacters, next: "generate_character_references",
		summary: "Characters: ready — 2 characters have no picture: Mama, Pip. Next: Make pictures for all characters.",
		lines:   map[string]string{stageCharacters: "characters: ready 0/2 — 2 characters have no picture: Mama, Pip"},
		decided: []string{"language"}},
	{name: "characters without voices", build: func(t *testing.T, s *Server, project string) {
		confirmBrief(s, project)
		characterWithSheet(t, s, project, "Pip")
		characterWithSheet(t, s, project, "Mama")
	}, current: stageScript, next: "generate_script", summary: "Story & scenes: ready — no scenes written yet. Next: Write the story.",
		lines: map[string]string{stageCharacters: "characters: done 2/2", stageVoices: "voices: ready (optional) 0/2 — no saved voice for Mama; no saved voice for Pip"}},
	{name: "scenes ready to render", build: func(t *testing.T, s *Server, project string) {
		confirmBrief(s, project)
		renderableScene(t, s, nil, "Opening", 1, "shots")
		renderableScene(t, s, nil, "Ending", 2, "shots")
	}, current: stageClips, next: "render_scenes",
		summary: "Scene videos: ready — 2 scenes have no video yet: “Opening”, “Ending”. Next: Make these videos.",
		lines:   map[string]string{stageShots: "shots: done 2/2", stageScenePictures: "scene_pictures: done 2/2", stageClips: "clips: ready 0/2 — 2 scenes have no video yet: “Opening”, “Ending”"}},
	{name: "clips rendered, not edited", build: renderedStory, current: stageEdit, next: "finish_video",
		summary: "Edit: ready — the scene videos are not joined into one video yet. Next: Join the scenes and export.",
		lines:   map[string]string{stageClips: "clips: done 2/2", stageReview: "review: ready (optional)", stageEdit: "edit: ready — the scene videos are not joined", stageExport: "export: not_started"}},
	{name: "edited, not exported", build: func(t *testing.T, s *Server, project string) {
		renderedStory(t, s, project)
		savedEdit(t, s, project, newestClip(t, s, project).Add(time.Minute))
	}, current: stageExport, next: "open_editor",
		summary: "Export: ready — the latest edit has not been exported. Next: Export the video.",
		lines:   map[string]string{stageEdit: "edit: done", stageReview: "review: done (optional)", stageExport: "export: ready — the latest edit has not been exported"}},
	{name: "exported", build: func(t *testing.T, s *Server, project string) {
		renderedStory(t, s, project)
		var outputs []string
		jobs, _ := s.database.ListRenderJobs(project)
		for _, j := range jobs {
			outs, _ := s.database.ListOutputsForJob(j.ID)
			for _, o := range outs {
				outputs = append(outputs, o.ID)
			}
		}
		savedEdit(t, s, project, newestClip(t, s, project))
		exported(t, s, project, outputs...)
	}, current: stageExport, next: "", summary: "Every stage is done: the final video is exported.",
		lines: map[string]string{stageEdit: "edit: done", stageExport: "export: done"}},
}

// evalContext is the part of the turn's project context this test reads.
type evalContext struct {
	Stages struct {
		Current  string       `json:"current"`
		Complete bool         `json:"complete"`
		Summary  string       `json:"summary"`
		Next     *stageAction `json:"next"`
		Lines    []string     `json:"stages"`
	} `json:"stages"`
	Decided struct {
		Rule      string              `json:"rule"`
		Decisions []map[string]string `json:"decisions"`
	} `json:"decided_so_far"`
	Mode string `json:"assistant_mode"`
}

// stageLine is the compact line of stage id.
func stageLine(lines []string, id string) string {
	for _, line := range lines {
		if strings.HasPrefix(line, id+": ") {
			return line
		}
	}
	return ""
}

func TestChatPromptCarriesEachStagesFacts(t *testing.T) {
	modes := []struct {
		name, body string
		// has and hasNot are phrases of the mode's rules.
		has, hasNot string
	}{
		{"manual", ``, "manual: every step waits", "full auto"},
		{"auto", `,"auto":true`, "auto: free steps", "full auto"},
		{"full", `,"auto":true,"full":true`, "The only questions allowed are the one plan confirmation", "manual:"},
	}
	for _, fx := range evalFixtures {
		t.Run(fx.name, func(t *testing.T) {
			s := evalServer(t)
			project, _ := s.database.GetActiveProjectID()
			fx.build(t, s, project)
			stages, err := s.projectStages(t.Context(), project)
			if err != nil {
				t.Fatal(err)
			}
			answer := `{"reply":"ok","action":"none","decision":{"mode":"inform"}}`
			system, prompt := plannerSays(s, &answer)
			for _, mode := range modes {
				w := call(t, s, "POST", "/chat", `{"message":"what's next?"`+mode.body+`}`)
				if w.Code != 200 {
					t.Fatalf("%s: %d %s", mode.name, w.Code, w.Body.String())
				}
				_, raw, found := strings.Cut(*prompt, "Project context: ")
				var ctx evalContext
				if !found || json.Unmarshal([]byte(raw), &ctx) != nil {
					t.Fatalf("%s: the prompt carries no readable project context", mode.name)
				}

				// The stage map: the turn carries GET /stages, compacted.
				next := ""
				if ctx.Stages.Next != nil {
					next = ctx.Stages.Next.Action
				}
				if ctx.Stages.Current != fx.current || next != fx.next || ctx.Stages.Current != stages.Current || ctx.Stages.Summary != stages.Summary || ctx.Stages.Complete != stages.Complete {
					t.Fatalf("%s: stages current %q next %q summary %q; want %q %q (GET /stages: %+v)", mode.name, ctx.Stages.Current, next, ctx.Stages.Summary, fx.current, fx.next, stages)
				}
				if len(ctx.Stages.Lines) != len(stageOrder) {
					t.Fatalf("%s: stage lines %v", mode.name, ctx.Stages.Lines)
				}
				for id, want := range fx.lines {
					if line := stageLine(ctx.Stages.Lines, id); !strings.HasPrefix(line, want) {
						t.Errorf("%s: stage %s line %q, want it to start %q", mode.name, id, line, want)
					}
				}
				if ctx.Stages.Summary != fx.summary {
					t.Errorf("%s: summary %q, want %q", mode.name, ctx.Stages.Summary, fx.summary)
				}

				// The decision log, with its rule, as GET /decisions has it.
				logged, _ := s.database.ListDecisions(project)
				if ctx.Decided.Rule != decidedSoFarRule || len(ctx.Decided.Decisions) != len(logged) {
					t.Fatalf("%s: decided_so_far %+v, log %+v", mode.name, ctx.Decided, logged)
				}
				carried := map[string]string{}
				for _, d := range ctx.Decided.Decisions {
					carried[d["key"]] = d["by"]
				}
				for _, key := range fx.decided {
					if carried[key] != db.DecisionByUser {
						t.Errorf("%s: decided_so_far misses %s (by user): %+v", mode.name, key, ctx.Decided.Decisions)
					}
				}

				// The mode's rules.
				if !strings.Contains(ctx.Mode, mode.has) || strings.Contains(ctx.Mode, mode.hasNot) {
					t.Errorf("%s: assistant_mode %q", mode.name, ctx.Mode)
				}

				// Size and secrets.
				if n := len(*prompt); n > maxChatPromptBytes {
					t.Errorf("%s: the turn's prompt is %d bytes, over the %d budget", mode.name, n, maxChatPromptBytes)
				}
				if n := len(*system); n > maxChatSystemBytes {
					t.Errorf("%s: the standing instructions are %d bytes, over the %d budget", mode.name, n, maxChatSystemBytes)
				}
				for _, text := range []string{*system, *prompt} {
					if leak := secretShapes.FindString(text); leak != "" {
						t.Fatalf("%s: the planner was given a secret-like value %q", mode.name, leak)
					}
				}
				t.Logf("%s: prompt %d bytes, system %d bytes", mode.name, len(*prompt), len(*system))
			}
		})
	}
}
