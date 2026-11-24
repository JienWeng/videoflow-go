package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
)

func TestSoundPlanChecks(t *testing.T) {
	scenes := map[string]bool{"a": true, "b": true}
	audio := func(id string) bool { return id == "asset_track" }
	good := soundPlan{Music: soundMusic{Mode: musicBed, Prompt: "slow piano"}, Ambience: []sceneAmbience{{SceneID: "a", Ambience: "rain on a tin roof"}}, ClipAudio: clipDialogueAmbience}
	if err := checkSoundPlan(good, 3, scenes, audio); err != nil {
		t.Fatal(err)
	}
	single := soundPlan{Music: soundMusic{Mode: musicInClip, Prompt: "slow piano"}, ClipAudio: clipWithMusic}
	if err := checkSoundPlan(single, 1, scenes, audio); err != nil {
		t.Fatalf("a single clip may make its own music: %v", err)
	}
	for name, mutate := range map[string]func(*soundPlan){
		"music in each of several clips": func(p *soundPlan) { p.Music.Mode, p.ClipAudio = musicInClip, clipWithMusic },
		"clips with music, bed mode":     func(p *soundPlan) { p.ClipAudio = clipWithMusic },
		"unknown mode":                   func(p *soundPlan) { p.Music.Mode = "loud" },
		"bed with nothing to play":       func(p *soundPlan) { p.Music.Prompt = "" },
		"none with a direction":          func(p *soundPlan) { p.Music.Mode = musicNone },
		"ambience brings music back":     func(p *soundPlan) { p.Ambience[0].Ambience = "rain and a soft piano" },
		"ambience of another story":      func(p *soundPlan) { p.Ambience[0].SceneID = "z" },
		"ambience quoting speech":        func(p *soundPlan) { p.Ambience[0].Ambience = `a voice says "hi"` },
		"track that is not audio":        func(p *soundPlan) { p.Music.Source = "asset_picture" },
	} {
		p := good
		p.Ambience = append([]sceneAmbience{}, good.Ambience...)
		mutate(&p)
		if checkSoundPlan(p, 3, scenes, audio) == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestDefaultSoundPlan(t *testing.T) {
	if p := defaultSoundPlan(3, "warm guitar"); p.Music.Mode != musicBed || p.ClipAudio != clipDialogueAmbience || p.musicInClip() {
		t.Errorf("several clips: %+v", p)
	}
	if p := defaultSoundPlan(1, "warm guitar"); p.Music.Mode != musicInClip || !p.musicInClip() {
		t.Errorf("one clip keeps its own music: %+v", p)
	}
	if p := defaultSoundPlan(3, ""); p.Music.Mode != musicNone || p.musicInClip() {
		t.Errorf("no music asked: %+v", p)
	}
}

// soundStory is a story of two scenes, made in two clips, whose brief asks for music.
func soundStory(t *testing.T, s *Server) (project string, script *models.Script, one, two *models.Scene) {
	t.Helper()
	project, _ = s.database.GetActiveProjectID()
	script = &models.Script{ProjectID: &project, Idea: "rain", Title: "Rain", Summary: "two friends wait out the rain", DraftJSON: []byte(`{"title":"Rain"}`)}
	if e := s.database.CreateScript(script); e != nil {
		t.Fatal(e)
	}
	one = renderableScene(t, s, &script.ID, "One", 1, "auto")
	two = renderableScene(t, s, &script.ID, "Two", 2, "auto")
	for _, sc := range []*models.Scene{one, two} {
		var doc map[string]any
		json.Unmarshal(sc.SceneJSON, &doc)
		doc["generation_brief"] = map[string]any{"idea": "rain", "music": "warm acoustic guitar"}
		sc.SceneJSON = voiceJSON(doc)
		if e := s.database.UpdateScene(sc); e != nil {
			t.Fatal(e)
		}
	}
	return
}

func TestMultiClipStoryClipsMakeNoMusicAndKeepTheSceneAmbience(t *testing.T) {
	s, _ := falServer(t, "done")
	project, script, one, two := soundStory(t, s)
	plan, clips := s.storySound(context.Background(), project, script.ID)
	if clips != 2 || plan.Music.Mode != musicBed || plan.Music.Prompt != "warm acoustic guitar" || plan.DecidedBy != decidedDefault {
		t.Fatalf("clips %d default plan %+v", clips, plan)
	}
	body := fmt.Sprintf(`{"music":{"mode":"bed","prompt":"warm acoustic guitar"},"ambience":[{"scene_id":%q,"ambience":"steady rain on a tin roof, distant traffic"}],"clip_audio":"dialogue_and_ambience"}`, one.ID)
	if w := call(t, s, "PUT", "/scripts/"+script.ID+"/sound-plan", body); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var draft map[string]any
	scripts, _ := s.database.ListScripts(project)
	for _, sc := range scripts {
		if sc.ID == script.ID {
			json.Unmarshal(sc.DraftJSON, &draft)
		}
	}
	if draft["title"] != "Rain" || draft["sound_plan"] == nil {
		t.Fatalf("the plan is not kept with the story: %v", draft)
	}
	first := sceneClipPrompts(t, s, one.ID, false)
	second := sceneClipPrompts(t, s, two.ID, false)
	if len(first) == 0 || len(second) == 0 {
		t.Fatal("no clips")
	}
	for _, p := range []string{first[0].Prompt, second[0].Prompt} {
		if strings.Contains(p, "guitar") || !strings.Contains(p, "No BGM; generate only voices, environmental sounds and action sounds.") {
			t.Errorf("a clip of a multi-clip story would make music:\n%s", p)
		}
	}
	if !strings.Contains(first[0].Prompt, "steady rain on a tin roof, distant traffic") {
		t.Errorf("the scene's ambience is missing:\n%s", first[0].Prompt)
	}

	refused := `{"music":{"mode":"clip","prompt":"warm acoustic guitar"},"clip_audio":"dialogue_ambience_and_music"}`
	if w := call(t, s, "PUT", "/scripts/"+script.ID+"/sound-plan", refused); w.Code != 422 || !strings.Contains(w.Body.String(), "restarts at every cut") {
		t.Fatalf("music made in each of two clips was accepted: %d %s", w.Code, w.Body.String())
	}
	var view soundPlanView
	w := call(t, s, "GET", "/scripts/"+script.ID+"/sound-plan", "")
	if json.Unmarshal(w.Body.Bytes(), &view) != nil || view.Plan.DecidedBy != decidedOwner || !strings.Contains(view.Line, "music: one bed under the whole film") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestAISoundPlanIsKeptOnlyWithinTheChecks(t *testing.T) {
	s, _ := falServer(t, "done")
	project, script, one, _ := soundStory(t, s)
	asked := 0
	answer := soundPlan{Music: soundMusic{Mode: musicBed, Prompt: "warm acoustic guitar"}, Ambience: []sceneAmbience{{SceneID: one.ID, Ambience: "rain with a soft piano"}}, ClipAudio: clipDialogueAmbience}
	previous := askSoundPlan
	askSoundPlan = func(_ *Server, _ context.Context, system, facts string) (soundPlan, error) {
		asked++
		if !strings.Contains(system, "never ask the clips for music") || !strings.Contains(facts, `"clips":2`) {
			t.Errorf("the sound designer was not told the rules or the clips:\n%s\n%s", system, facts)
		}
		return answer, nil
	}
	t.Cleanup(func() { askSoundPlan = previous })

	if plan := s.decideSoundPlan(context.Background(), project, script.ID); plan.DecidedBy != decidedDefault || plan.ambienceFor(one.ID) != "" {
		t.Fatalf("ambience with music in it was kept: %+v", plan)
	}
	answer.Ambience[0].Ambience = "rain on the window, a kettle ticking"
	if plan := s.decideSoundPlan(context.Background(), project, script.ID); plan.DecidedBy != decidedAI || plan.ambienceFor(one.ID) != "rain on the window, a kettle ticking" {
		t.Fatalf("the AI's plan was not kept: %+v", plan)
	}
	s.decideSoundPlan(context.Background(), project, script.ID)
	if asked != 2 {
		t.Errorf("a decided plan was asked for again: %d", asked)
	}
}

func TestEditStageSaysWhoMakesTheSoundAndWhetherTheBedIsWhole(t *testing.T) {
	line := "Sound: the clips' own voices, ambience and action sounds (Gemini Omni); music: one bed under the whole film from your upload “rain.mp3”, lowered under speech"
	in := stageInputs{Sound: &stageSound{Line: line, WantsBed: true, Edit: &composition.Soundtrack{Music: true, Continuous: true, Ducks: 3, CutFades: 2}}}
	st := editStage(in)
	want := line + "; in the edit: one music bed, continuous, lowered under speech 3 times; 2 cuts crossfaded"
	if st.Sound != want {
		t.Fatalf("sound line\n got %q\nwant %q", st.Sound, want)
	}
	chat := chatStages(stageMap{Stages: []projectStage{st}})
	if lines := chat["stages"].([]string); !strings.Contains(lines[0], "in the edit: one music bed, continuous") {
		t.Errorf("the assistant is not told: %v", lines)
	}
	in.Sound.Edit = &composition.Soundtrack{CutFades: 0}
	if got := editStage(in).Sound; !strings.HasSuffix(got, "no music bed yet (add music: upload a track in Edit video, or join the scenes again once it is made)") {
		t.Errorf("missing bed: %q", got)
	}
	in.Sound.Edit = nil
	if got := editStage(in).Sound; got != line {
		t.Errorf("before any edit: %q", got)
	}
}

// "What is the sound provider?" is answered from the episode's facts.
func TestAssistantIsToldWhoMakesTheSound(t *testing.T) {
	s, _ := falServer(t, "done")
	answer := `{"reply":"The clips make their own voices and ambience; music is one bed.","action":"none"}`
	system, prompt := plannerSays(s, &answer)
	soundStory(t, s)
	sendChat(t, s, "what is the sound provider")
	for _, want := range []string{`"sound":"Sound: the clips' own voices, ambience and action sounds (`, `music: one bed under the whole film (“warm acoustic guitar”), not made yet`, `"clip_audio":"dialogue_and_ambience"`, `"clips":2`} {
		if !strings.Contains(*prompt, want) {
			t.Errorf("the planner was not told %s:\n%.600s", want, (*prompt)[strings.Index(*prompt, "episode_sound"):])
		}
	}
	if !strings.Contains(*system, "`sound` is the plain answer to \"what is the sound provider?\"") {
		t.Error("the planner's rules do not say how to answer it")
	}
}
