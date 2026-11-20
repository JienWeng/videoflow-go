package api

import (
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
