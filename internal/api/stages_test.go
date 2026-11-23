package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"videoflow-go/internal/models"
	"videoflow-go/internal/speech"
)

func stageByID(t *testing.T, m stageMap, id string) projectStage {
	t.Helper()
	for _, st := range m.Stages {
		if st.ID == id {
			return st
		}
	}
	t.Fatalf("no stage %s in %+v", id, m)
	return projectStage{}
}

func progressAt(stage, video string) sceneProgress {
	return sceneProgress{Stage: stage, Video: sceneVideo{State: video}}
}

func TestStagesFollowTheProjectsSavedState(t *testing.T) {
	clip := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	allReady := []stageScene{
		{ID: "s1", Title: "Opening", Progress: progressAt("video_ready", "ready")},
		{ID: "s2", Title: "Ending", Progress: progressAt("video_ready", "ready")},
	}
	cast := []stageCharacter{{ID: "c1", Name: "Satoshi", HasPicture: true, HasVoice: true}}
	for _, tc := range []struct {
		name, current, action string
		in                    stageInputs
		check                 func(t *testing.T, m stageMap)
	}{
		{name: "empty project", current: stageBrief, action: "generate_script", in: stageInputs{}, check: func(t *testing.T, m stageMap) {
			if st := stageByID(t, m, stageBrief); st.Status != statusReady || st.Next.Cost.Class != costFree {
				t.Fatalf("brief: %+v", st)
			}
			for _, id := range []string{stageShots, stageClips, stageEdit, stageExport} {
				if st := stageByID(t, m, id); st.Status != statusNotStarted {
					t.Errorf("%s: %+v", id, st)
				}
			}
		}},
		{name: "characters without voices or pictures", current: stageCharacters, action: "generate_character_references", in: stageInputs{
			Characters: []stageCharacter{{ID: "c1", Name: "Satoshi", HasPicture: true}, {ID: "c2", Name: "Mika"}, {ID: "c3", Name: "Ren"}},
			Scenes:     []stageScene{{ID: "s1", Title: "Opening", Progress: progressAt("needs_references", "none")}},
		}, check: func(t *testing.T, m stageMap) {
			chars := stageByID(t, m, stageCharacters)
			if chars.Done != 1 || chars.Total != 3 || !strings.Contains(chars.Missing[0], "2 characters have no picture: Mika, Ren") {
				t.Fatalf("characters: %+v", chars)
			}
			if c := chars.Next.Cost; c.Class != costImage || c.ImageCalls != 2 || len(chars.Next.CharacterIDs) != 2 {
				t.Fatalf("characters cost: %+v", chars.Next)
			}
			voices := stageByID(t, m, stageVoices)
			if !voices.Optional || voices.Status != statusReady || strings.Join(voices.Missing, ",") != "no saved voice for Satoshi,no saved voice for Mika,no saved voice for Ren" {
				t.Fatalf("voices: %+v", voices)
			}
		}},
		{name: "voices alone never hold the project back", current: stageClips, action: "render_scene", in: stageInputs{
			Characters: []stageCharacter{{ID: "c1", Name: "Satoshi", HasPicture: true}},
			Scenes:     []stageScene{{ID: "s1", Title: "Opening", Progress: progressAt("ready_to_render", "none"), VideoSeconds: 8}},
		}},
		{name: "scenes ready to render", current: stageClips, action: "render_scenes", in: stageInputs{
			Characters: cast,
			Scenes: []stageScene{
				{ID: "s1", Title: "Opening", Progress: progressAt("ready_to_render", "none"), VideoSeconds: 8},
				{ID: "s2", Title: "Middle", Progress: sceneProgress{Stage: "needs_attention", Video: sceneVideo{State: "failed"}}, VideoSeconds: 6},
				{ID: "s3", Title: "Ending", Progress: progressAt("video_ready", "ready")},
			},
		}, check: func(t *testing.T, m stageMap) {
			clips := stageByID(t, m, stageClips)
			if clips.Status != statusReady || clips.Done != 1 || clips.Total != 3 {
				t.Fatalf("clips: %+v", clips)
			}
			if n := clips.Next; n.Cost.Class != costVideo || n.Cost.VideoSeconds != 14 || n.Reversible || strings.Join(n.SceneIDs, ",") != "s1,s2" {
				t.Fatalf("clips next: %+v", n)
			}
			for _, id := range []string{stageShots, stageScenePictures, stageBrief, stageScript} {
				if st := stageByID(t, m, id); st.Status != statusDone {
					t.Errorf("%s: %+v", id, st)
				}
			}
		}},
		{name: "a scene waiting on the service blocks the videos", current: stageClips, action: "check_video", in: stageInputs{
			Scenes: []stageScene{{ID: "s1", Title: "Opening", Progress: progressAt("needs_attention", "waiting_on_service")}, {ID: "s2", Title: "Ending", Progress: progressAt("ready_to_render", "none")}},
		}, check: func(t *testing.T, m stageMap) {
			if st := stageByID(t, m, stageClips); st.Status != statusBlocked {
				t.Fatalf("clips: %+v", st)
			}
		}},
		{name: "shots come first, then pictures", current: stageShots, action: "generate_shots", in: stageInputs{
			Scenes: []stageScene{{ID: "s1", Title: "Opening", Progress: progressAt("needs_shots", "none")}, {ID: "s2", Title: "Ending", Progress: progressAt("needs_storyboard", "none")}},
		}, check: func(t *testing.T, m stageMap) {
			pics := stageByID(t, m, stageScenePictures)
			if pics.Status != statusReady || pics.Next.Action != "storyboard" || pics.Next.Cost.ImageCalls != 1 {
				t.Fatalf("pictures: %+v", pics)
			}
		}},
		{name: "rendered but not edited", current: stageEdit, action: "finish_video", in: stageInputs{Characters: cast, Scenes: allReady, LastClipAt: clip}, check: func(t *testing.T, m stageMap) {
			if st := stageByID(t, m, stageReview); st.Status != statusReady || !st.Optional {
				t.Fatalf("review: %+v", st)
			}
			if st := stageByID(t, m, stageEdit); st.Next.Cost.Class != costFree || st.Status != statusReady {
				t.Fatalf("edit: %+v", st)
			}
			if st := stageByID(t, m, stageExport); st.Status != statusNotStarted {
				t.Fatalf("export: %+v", st)
			}
		}},
		{name: "an edit older than the newest clip is out of date", current: stageEdit, action: "finish_video", in: stageInputs{Scenes: allReady, LastClipAt: clip, LastEditAt: clip.Add(-time.Hour)}},
		{name: "edited, not exported", current: stageExport, action: "open_editor", in: stageInputs{Scenes: allReady, LastClipAt: clip, LastEditAt: clip.Add(time.Minute)}, check: func(t *testing.T, m stageMap) {
			if m.Next.EditorPanel != "export" {
				t.Fatalf("next: %+v", m.Next)
			}
		}},
		{name: "exported", current: stageExport, in: stageInputs{Characters: cast, Scenes: allReady, LastClipAt: clip, LastEditAt: clip.Add(time.Minute), LastExportAt: clip.Add(2 * time.Minute)}, check: func(t *testing.T, m stageMap) {
			if !m.Complete || m.Next != nil || !strings.Contains(m.Summary, "exported") {
				t.Fatalf("map: %+v", m)
			}
			for _, st := range m.Stages {
				if st.Status != statusDone {
					t.Errorf("%s: %+v", st.ID, st)
				}
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := computeStages(tc.in)
			if len(m.Stages) != len(stageOrder) {
				t.Fatalf("stages: %+v", m.Stages)
			}
			for i, st := range m.Stages {
				if st.ID != stageOrder[i] {
					t.Fatalf("order: %v", m.Stages)
				}
			}
			action := ""
			if m.Next != nil {
				action = m.Next.Action
			}
			if m.Current != tc.current || action != tc.action {
				t.Fatalf("current %s next %q, want %s %q: %+v", m.Current, action, tc.current, tc.action, m)
			}
			if tc.check != nil {
				tc.check(t, m)
			}
		})
	}
}

// The dialogue stage comes from each scene's speech-fit verdict: done when every
// speaking scene fits, ready with Fit script to clips (free, reversible) when one won't
// fit, optional when one only just fits, and never blocked.
func TestDialogueStageFollowsTheSpeechFit(t *testing.T) {
	ready := func(id, title, verdict, summary, over string) stageScene {
		return stageScene{ID: id, Title: title, Progress: progressAt("ready_to_render", "none"), VideoSeconds: 8, Speech: verdict, SpeechSummary: summary, SpeechWontFit: over}
	}
	for _, tc := range []struct {
		name, status, current, action string
		optional                      bool
		done, total                   int
		missing                       string
		scenes                        []stageScene
	}{
		{name: "every speaking scene fits", status: statusDone, current: stageClips, action: "render_scenes", done: 1, total: 1, scenes: []stageScene{
			ready("s1", "Opening", speech.OK, "Speech fits: 6 s of 8 s ✓", ""), ready("s2", "Quiet", "", "", ""),
		}},
		{name: "a scene that won't fit is the next step", status: statusReady, current: stageDialogue, action: "fit_script", done: 1, total: 3,
			missing: "speech won't fit in 1 scene — fit the script first: “Trust” (11.4 s of 8 s)|speech only just fits in “Code” (Speech fits, just: 7.5 s of 8 s)", scenes: []stageScene{
				ready("s1", "Opening", speech.OK, "Speech fits: 6 s of 8 s ✓", ""),
				ready("s2", "Trust", speech.TooLong, "Won't fit: 11.4 s of 8 s — fit the script first", "11.4 s of 8 s"),
				ready("s3", "Code", speech.Tight, "Speech fits, just: 7.5 s of 8 s", ""),
			}},
		{name: "only just fits: offered, never holding the project back", status: statusReady, optional: true, current: stageClips, action: "render_scene", total: 1,
			missing: "speech only just fits in “Code” (Speech fits, just: 7.5 s of 8 s)", scenes: []stageScene{ready("s3", "Code", speech.Tight, "Speech fits, just: 7.5 s of 8 s", "")}},
		{name: "made scenes are settled", status: statusDone, current: stageEdit, action: "finish_video", scenes: []stageScene{
			{ID: "s1", Title: "Opening", Progress: progressAt("video_ready", "ready")},
		}},
		{name: "waits for shots", status: statusNotStarted, current: stageShots, action: "generate_shots", missing: "1 scene needs shots first; the lines are sized to them", scenes: []stageScene{
			{ID: "s1", Title: "Opening", Progress: progressAt("needs_shots", "none")},
		}},
		{name: "fit before the pictures", status: statusReady, current: stageDialogue, action: "fit_script", total: 1,
			missing: "speech won't fit in 1 scene — fit the script first: “Trust” (Won't fit: 11 s of 8 s — fit the script first)", scenes: []stageScene{
				{ID: "s2", Title: "Trust", Progress: progressAt("needs_storyboard", "none"), Speech: speech.TooLong, SpeechSummary: "Won't fit: 11 s of 8 s — fit the script first"},
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := computeStages(stageInputs{Scenes: tc.scenes})
			st := stageByID(t, m, stageDialogue)
			if st.Status != tc.status || st.Optional != tc.optional || st.Done != tc.done || st.Total != tc.total || strings.Join(st.Missing, "|") != tc.missing {
				t.Fatalf("dialogue: %+v", st)
			}
			if st.Status == statusReady && (st.Next == nil || st.Next.Action != "fit_script" || st.Next.Cost.Class != costFree || !st.Next.Reversible || len(st.Next.SceneIDs) != 1) {
				t.Fatalf("next: %+v", st.Next)
			}
			action := ""
			if m.Next != nil {
				action = m.Next.Action
			}
			if m.Current != tc.current || action != tc.action {
				t.Fatalf("current %s next %q", m.Current, action)
			}
			for _, line := range chatStages(m)["stages"].([]string) {
				if strings.HasPrefix(line, "clips:") && strings.Contains(line, "speech") {
					t.Fatalf("the videos stage repeats the speech: %s", line)
				}
			}
		})
	}
}

// GET /stages reads saved work: characters, their voices, scenes and their videos.
func TestStagesEndpointReadsTheActiveProject(t *testing.T) {
	s := routerServer(t, nil)
	project, _ := s.database.GetActiveProjectID()
	var m stageMap
	if w := call(t, s, "GET", "/stages", ""); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &m) != nil || m.Current != stageBrief {
		t.Fatalf("empty project: %d %s", w.Code, w.Body.String())
	}

	for _, name := range []string{"Satoshi", "Mika"} {
		if w := call(t, s, "POST", "/characters", `{"name":"`+name+`"}`); w.Code != 201 && w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	chars, _ := s.database.ListCharacters(project)
	profile, _ := json.Marshal(VoiceProfile{CharacterID: chars[1].ID, Provider: "fish", VoiceID: "voice123", Approved: true})
	if _, err := s.database.SaveVoiceDocument(project, "profile", chars[1].ID, 0, profile); err != nil {
		t.Fatal(err)
	}
	scenes := storyScenes(t, s, "Opening", "Ending")
	if err := s.database.CreateShot(&models.Shot{SceneID: scenes[1].ID, ShotOrder: 1, Duration: 4, Prompt: "wide"}); err != nil {
		t.Fatal(err)
	}
	sceneJob(t, s, scenes[1], "succeeded", "")

	w := call(t, s, "GET", "/stages", "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &m) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	if m.Current != stageCharacters || m.Next.Action != "generate_character_references" || m.Next.Cost.ImageCalls != 2 {
		t.Fatalf("map: %s", w.Body.String())
	}
	voices := stageByID(t, m, stageVoices)
	if voices.Done != 1 || len(voices.Missing) != 1 || !strings.Contains(voices.Missing[0], chars[0].Name) {
		t.Fatalf("voices: %+v", voices)
	}
	if st := stageByID(t, m, stageShots); st.Done != 1 || st.Total != 2 {
		t.Fatalf("shots: %+v", st)
	}
	if st := stageByID(t, m, stageClips); st.Done != 1 {
		t.Fatalf("clips: %+v", st)
	}
}

// Every chat turn carries the compact stage map, and the trimmed progress keeps what
// the planner's actions need.
func TestChatTurnCarriesTheStageMap(t *testing.T) {
	answer := `{"reply":"ok","action":"none"}`
	s := routerServer(t, nil)
	_, prompt := plannerSays(s, &answer)
	storyScenes(t, s, "Opening")
	sendChat(t, s, "what next?")
	_, raw, _ := strings.Cut(*prompt, "Project context: ")
	var context struct {
		Stages struct {
			Current string   `json:"current"`
			Lines   []string `json:"stages"`
			Next    *stageAction
		} `json:"stages"`
		Progress map[string]any `json:"production_progress"`
	}
	if err := json.Unmarshal([]byte(raw), &context); err != nil {
		t.Fatal(err)
	}
	if context.Stages.Current != stageShots || len(context.Stages.Lines) != len(stageOrder) || context.Stages.Next == nil || context.Stages.Next.Action != "generate_shots" {
		t.Fatalf("stages: %+v", context.Stages)
	}
	if !strings.HasPrefix(context.Stages.Lines[0], "brief: done") || !strings.Contains(context.Stages.Lines[4], "no shots") {
		t.Fatalf("lines: %v", context.Stages.Lines)
	}
	if _, long := context.Progress["summary"]; long || strings.Contains(raw, "Break this scene into camera shots") {
		t.Fatal("progress still carries the long help texts")
	}
}
