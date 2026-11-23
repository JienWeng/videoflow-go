package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"videoflow-go/internal/models"
	"videoflow-go/internal/speech"
)

// The render plan says, per clip, whether the speech fits, before anything is paid for;
// only a clip that makes its own sound is held back by speech that won't fit.
func TestProductionPlanChecksSpeechPerClip(t *testing.T) {
	s, scene, _, _ := fitScene(t)
	plan, err := s.sceneProductionPlan(context.Background(), scene)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Speech == nil || plan.Speech.Verdict != speech.TooLong || !strings.HasPrefix(plan.speechWontFit(), "Won't fit: ") || !strings.HasSuffix(plan.speechWontFit(), "of 8 s — fit the script first") {
		t.Fatalf("speech: %+v / %q", plan.Speech, plan.speechWontFit())
	}
	if got := plan.Speech.overrun(); !strings.HasSuffix(got, " of 8 s") {
		t.Fatalf("overrun = %q", got)
	}
	plan.Settings.AudioMode = "silent"
	if plan.speechWontFit() != "" {
		t.Fatal("silent clips are not held back by their script")
	}
}

func TestStagesSayWhenSpeechWontFit(t *testing.T) {
	in := stageInputs{Scenes: []stageScene{
		{ID: "s1", Title: "Trust", Progress: progressAt("ready_to_render", "none"), VideoSeconds: 8, Speech: speech.TooLong, SpeechSummary: "Won't fit: 11.4 s of 8 s — fit the script first", SpeechWontFit: "11.4 s of 8 s"},
		{ID: "s2", Title: "Code", Progress: progressAt("ready_to_render", "none"), VideoSeconds: 8, Speech: speech.OK, SpeechSummary: "Speech fits: 6 s of 8 s ✓"},
	}}
	m := computeStages(in)
	dialogue := stageByID(t, m, stageDialogue)
	if len(dialogue.Missing) != 1 || dialogue.Missing[0] != "speech won't fit in 1 scene — fit the script first: “Trust” (11.4 s of 8 s)" {
		t.Fatalf("missing: %q", dialogue.Missing)
	}
	// The videos stage no longer repeats it: the dialogue stage owns the speech.
	if clips := stageByID(t, m, stageClips); len(clips.Missing) != 1 || strings.Contains(clips.Missing[0], "speech") {
		t.Fatalf("clips: %q", clips.Missing)
	}
	fit := chatSpeechFit(in.Scenes)
	if len(fit) != 1 || fit[0]["scene_id"] != "s1" || !strings.Contains(fit[0]["summary"], "11.4 s of 8 s") {
		t.Fatalf("chat speech fit: %+v", fit)
	}
}

// Full auto fits a scene's script itself before its video is paid for, and stops the run
// when the speech still won't fit; a run started by a click goes ahead.
func TestFullAutoFitsTheScriptBeforeRendering(t *testing.T) {
	s, scene, _, mei := fitScene(t)
	project := *scene.ProjectID
	op := &models.Op{Kind: "autopilot", ProjectID: &project, Status: "running"}
	if err := s.database.CreateOp(op); err != nil {
		t.Fatal(err)
	}
	calls := 0
	answer := `{"lines":[{"id":"l1","character_id":"` + mei.ID + `","text":"` + strings.Repeat("钱是什么", 12) + `","shot":1}]}`
	s.agents.SetStructuredRunner(func(_ context.Context, _, _, _ string, target any) error {
		calls++
		return json.Unmarshal([]byte(answer), target)
	})
	run := func(full bool) error {
		fresh, _ := s.database.GetScene(scene.ID)
		a := &autopilotRunner{s: s, op: op, project: project, body: autopilotRequest{SceneIDs: []string{scene.ID}, VideosOnly: true, FullAuto: full}, scenes: []models.Scene{*fresh}}
		return a.fitScripts(context.Background(), nil)
	}
	if err := run(false); err != nil || calls != 0 {
		t.Fatalf("a clicked run fits nothing: %v, %d calls", err, calls)
	}
	err := run(true)
	if err == nil || !strings.Contains(err.Error(), "still has more speech than its clip can hold") || !strings.Contains(err.Error(), "no video was paid for") || calls != 1 {
		t.Fatalf("still too long: %v, %d calls", err, calls)
	}
	answer = `{"lines":[{"id":"l1","character_id":"` + mei.ID + `","text":"钱是什么？","shot":1},{"id":"l2","character_id":"` + mei.ID + `","text":"谁来保管？","shot":3}]}`
	if err = run(true); err != nil || calls != 2 {
		t.Fatalf("fitted: %v, %d calls", err, calls)
	}
	saved, _ := s.database.GetScene(scene.ID)
	if lines := sceneSpokenLines(saved); len(lines) != 2 || lines[0].Text != "钱是什么？" {
		t.Fatalf("fitted lines not saved: %+v", lines)
	}
	if jobs, _ := s.database.ListRenderJobs(project); len(jobs) != 0 {
		t.Fatalf("fitting must not start a video: %d jobs", len(jobs))
	}
}
