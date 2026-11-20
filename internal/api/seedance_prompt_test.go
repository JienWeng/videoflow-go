package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"videoflow-go/internal/models"
)

// What a planned scene clip asks the video service for: the scene's own
// shots, cast and lines in the guide's form, and none of the app's internal records.
func TestSceneClipIsSentAGuideShapedPrompt(t *testing.T) {
	fastAutopilot(t)
	s, _ := falServer(t, "done")
	project, _ := s.database.GetActiveProjectID()
	hero := &models.Character{ProjectID: &project, Name: "Chen Wei", Appearance: "East Asian man in his late thirties with short black hair. Wears an olive work jacket."}
	if e := s.database.CreateCharacter(hero); e != nil {
		t.Fatal(e)
	}
	scene := renderableScene(t, s, nil, "The Quiet End", 1, "auto")
	scene.Summary = "Chen Wei watches the city go quiet"
	scene.SceneJSON = voiceJSON(map[string]any{
		"production":       productionSettings{AudioMode: "native", Strategy: "auto", MusicPrompt: "slow sparse piano"},
		"generation_brief": map[string]any{"idea": "the end", "character_ids": []string{hero.ID}, "conversation_mode": "narration", "language": "Mandarin", "tone": "Calm, sorrowful"},
		"spoken_lines":     []SpokenLine{{ID: "l1", Language: "Mandarin", Text: "人类的终结，没有爆炸。"}},
	})
	if e := s.database.UpdateScene(scene); e != nil {
		t.Fatal(e)
	}
	// The scene changed, so point its storyboard at the scene as it is now.
	revision, _ := s.sceneInputRevision(scene)
	assets, _ := s.database.ListAssets(project)
	for i := range assets {
		if assets[i].Type == "storyboard" {
			assets[i].MetadataJSON = voiceJSON(map[string]any{"scene_id": scene.ID, "scene_input_revision": revision, "consistency_review": "ready"})
			if e := s.database.UpdateAsset(&assets[i]); e != nil {
				t.Fatal(e)
			}
		}
	}

	var preview struct {
		Clips []struct{ Prompt string }
	}
	if w := call(t, s, "GET", "/scenes/"+scene.ID+"/video-prompt", ""); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &preview) != nil || len(preview.Clips) != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call(t, s, "POST", "/autopilot/finish?background=true", fmt.Sprintf(`{"scene_ids":[%q],"videos_only":true}`, scene.ID)); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	// The request waiting for the worker carries the prompt the video service will get.
	<-finishPendingVideos(t, s, project, 1)
	jobs, _ := s.database.ListRenderJobs(project)
	var request struct {
		Final string `json:"final_prompt"`
	}
	if len(jobs) != 1 || json.Unmarshal(jobs[0].RequestJSON, &request) != nil {
		t.Fatalf("jobs: %d", len(jobs))
	}
	sent := request.Final
	if sent != preview.Clips[0].Prompt {
		t.Fatalf("the preview and the request differ.\nrequest:\n%s\npreview:\n%s", sent, preview.Clips[0].Prompt)
	}
	for _, want := range []string{"Chen Wei watches the city go quiet", "Chen Wei: East Asian man", "Shot 1: Action 1", "Shot 3: Action 3", "Voice-over: {人类的终结，没有爆炸。}", "one unseen narrator", "(slow sparse piano plays quietly", "No subtitles"} {
		if !strings.Contains(sent, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	for _, leak := range []string{hero.ID, scene.ID, project, `{"`, "Authoritative", "audience", "best effort"} {
		if strings.Contains(sent, leak) {
			t.Errorf("prompt leaks %q", leak)
		}
	}
	if strings.Count(sent, "人类的终结，没有爆炸。") != 1 || len(sent) > 1500 {
		t.Errorf("line count %d, length %d", strings.Count(sent, "人类的终结，没有爆炸。"), len(sent))
	}
	if t.Failed() {
		t.Log("\n" + sent)
	}
}

// The AI prompt writer's prompt is used when it keeps the scene's facts, remembered so
// the same clip is written once, and replaced by the plain draft when it does not.
func TestAIWrittenPromptIsUsedOnlyWhenItKeepsTheFacts(t *testing.T) {
	s, _ := falServer(t, "done")
	scene := renderableScene(t, s, nil, "One", 1, "auto")
	asked, answer := 0, ""
	previous := writeVideoPrompt
	t.Cleanup(func() { writeVideoPrompt = previous })
	writeVideoPrompt = func(_ *Server, _ context.Context, system, facts string) (string, error) {
		asked++
		if !strings.Contains(system, "Do NOT put second counts or timestamps on shots") || !strings.Contains(facts, `"what_happens": "Action 2"`) {
			t.Errorf("the writer was not given the playbook and the scene's shots")
		}
		return answer, nil
	}
	read := func(query string) (prompt, by string) {
		t.Helper()
		var out struct {
			Clips []struct {
				Prompt string
				By     string `json:"written_by"`
			}
		}
		if w := call(t, s, "GET", "/scenes/"+scene.ID+"/video-prompt"+query, ""); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.Clips) != 1 {
			t.Fatal(w.Code, w.Body.String())
		}
		return out.Clips[0].Prompt, out.Clips[0].By
	}

	if _, by := read(""); by != "template" || asked != 0 {
		t.Fatalf("a plain look asked the writer: by=%s asked=%d", by, asked)
	}
	answer = "Shot 1: Action 1. Shot 2: Action 2. No BGM. Keep it subtitle-free."
	if _, by := read("?write=1"); by != "template" {
		t.Fatalf("a prompt that dropped shot 3 was accepted")
	}
	answer = "A calm morning. Shot 1: wide shot, Action 1. Shot 2: slow push-in, Action 2. Shot 3: close-up, Action 3. No BGM; only natural sound. Keep it subtitle-free."
	if prompt, by := read("?write=1"); by != "ai" || prompt != answer {
		t.Fatalf("by=%s prompt=%q", by, prompt)
	}
	before := asked
	if prompt, by := read(""); by != "ai" || prompt != answer || asked != before {
		t.Fatalf("the written prompt was not remembered: by=%s asked %d times more", by, asked-before)
	}
}
