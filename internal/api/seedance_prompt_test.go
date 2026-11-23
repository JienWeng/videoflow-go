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

	preview := sceneClipPrompts(t, s, scene.ID, false)
	if len(preview) != 1 {
		t.Fatal(preview)
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
	if sent != preview[0].Prompt {
		t.Fatalf("the preview and the request differ.\nrequest:\n%s\npreview:\n%s", sent, preview[0].Prompt)
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
		out := sceneClipPrompts(t, s, scene.ID, query == "?write=1")
		if len(out) != 1 {
			t.Fatal(out)
		}
		return out[0].Prompt, out[0].By
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
	// A refused prompt gets one repair, with the reason, before the draft is used.
	scene.Summary = "A different morning"
	s.database.UpdateScene(scene)
	repair := ""
	writeVideoPrompt = func(_ *Server, _ context.Context, _, facts string) (string, error) {
		if strings.Contains(facts, "Your previous prompt was refused: shot 3 is missing") {
			repair = facts
			return answer, nil
		}
		return "Shot 1: Action 1. Shot 2: Action 2. No BGM. Keep it subtitle-free.", nil
	}
	revision, _ := s.sceneInputRevision(scene)
	project, _ := s.database.GetActiveProjectID()
	boards, _ := s.database.ListAssets(project)
	for i := range boards {
		if boards[i].Type == "storyboard" {
			boards[i].MetadataJSON = voiceJSON(map[string]any{"scene_id": scene.ID, "scene_input_revision": revision, "consistency_review": "ready"})
			s.database.UpdateAsset(&boards[i])
		}
	}
	if prompt, by := read("?write=1"); by != "ai" || prompt != answer || repair == "" {
		t.Fatalf("the writer was not given a chance to repair: by=%s", by)
	}
	asked = 0
	writeVideoPrompt = func(_ *Server, _ context.Context, _, _ string) (string, error) { asked++; return answer, nil }
	before := asked
	if prompt, by := read(""); by != "ai" || prompt != answer || asked != before {
		t.Fatalf("the written prompt was not remembered: by=%s asked %d times more", by, asked-before)
	}
}

// clipPrompt is the prompt one clip of a scene would be sent with right now.
type clipPrompt struct {
	Prompt   string
	By       string
	Pictures []referencePicture
}

// sceneClipPrompts writes, without making or sending any video, the prompt of each clip
// in a scene's render plan, the way a render writes it. With write on, the AI prompt
// writer is asked; otherwise only a remembered prompt or the plain draft is used.
func sceneClipPrompts(t *testing.T, s *Server, sceneID string, write bool) []clipPrompt {
	t.Helper()
	ctx := context.Background()
	scene, err := s.database.GetScene(sceneID)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := s.sceneProductionPlan(ctx, scene)
	if err != nil {
		t.Fatal(err)
	}
	clips := []clipPrompt{}
	for _, unit := range plan.Units {
		job := sceneRenderJob(*scene.ProjectID, scene)
		job.Provider, job.Model = plan.Provider, plan.Model
		request := map[string]any{"render_unit": unit, "music_prompt": plan.Settings.MusicPrompt, "voice_policy": map[string]any{"mode": plan.Settings.AudioMode}, "video_input": plan.VideoInput}
		job.RequestJSON = voiceJSON(request)
		if plan.VideoInput == inputReferences {
			if _, model, e := s.productionVideoModel(ctx); e == nil {
				_ = s.attachReferencePictures(job, request, model.ReferenceImages)
				job.RequestJSON = voiceJSON(request)
			}
		}
		s.writeScenePrompt(ctx, job, write)
		var written struct {
			Prompt   string             `json:"final_prompt"`
			By       string             `json:"prompt_writer"`
			Pictures []referencePicture `json:"reference_pictures"`
		}
		if json.Unmarshal(job.RequestJSON, &written) != nil {
			t.Fatal(string(job.RequestJSON))
		}
		clips = append(clips, clipPrompt{written.Prompt, written.By, written.Pictures})
	}
	return clips
}

// A Gemini Omni clip is told, per shot, who says what and when; lines saved for the
// whole scene are placed by time. A Seedance clip of the same scene keeps its notation.
func TestOmniClipPromptPlacesEachLineInItsShot(t *testing.T) {
	s, scene := productionFixture(t, "auto")
	project := *scene.ProjectID
	mei := &models.Character{ProjectID: &project, Name: "Mei", Appearance: "Young woman in a grey coat"}
	jun := &models.Character{ProjectID: &project, Name: "Jun", Appearance: "Older man with glasses"}
	for _, c := range []*models.Character{mei, jun} {
		if e := s.database.CreateCharacter(c); e != nil {
			t.Fatal(e)
		}
	}
	shots, _ := s.database.ListShots(scene.ID)
	scene.CharacterIDsJSON = voiceJSON([]string{mei.ID, jun.ID})
	scene.SceneJSON = voiceJSON(map[string]any{
		"production":       productionSettings{AudioMode: "native", Strategy: "auto"},
		"generation_brief": map[string]any{"language": "Chinese"},
		"spoken_lines": []SpokenLine{
			{ID: "l1", CharacterID: mei.ID, Language: "Chinese", Text: "如果钱不需要靠银行来信任呢？", Visible: true},
			{ID: "l2", CharacterID: jun.ID, Language: "zh", Text: "那就用代码。", Visible: true},
			{ID: "l3", CharacterID: mei.ID, Language: "Chinese", Text: "我们试试吧。", Visible: true, ShotID: shots[2].ID},
		},
	})
	if e := s.database.UpdateScene(scene); e != nil {
		t.Fatal(e)
	}
	ids := []string{shots[0].ID, shots[1].ID, shots[2].ID}
	prompt := func(provider, model string) string {
		job := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Provider: provider, Model: model, RequestJSON: voiceJSON(map[string]any{"render_unit": renderUnit{ShotIDs: ids, Length: 8, Count: 1}, "voice_policy": map[string]any{"mode": "native"}})}
		s.writeScenePrompt(context.Background(), job, false)
		var payload map[string]any
		_ = json.Unmarshal(job.RequestJSON, &payload)
		text, _ := payload[finalPromptKey].(string)
		return text
	}
	omni := prompt("gemini", "gemini-omni-1.2-flash")
	for _, want := range []string{
		"Shot 1 (0-3s): Action 1.\nAt 0s, Mei says in Chinese: \"如果钱不需要靠银行来信任呢？\"",
		"Shot 2 (3-5s): Action 2.\nAt 3.5s, Jun says in Chinese: \"那就用代码。\"",
		"Shot 3 (5-8s): Action 3.\nAt 5s, Mei says in Chinese: \"我们试试吧。\"",
	} {
		if !strings.Contains(omni, want) {
			t.Errorf("Omni prompt lacks %q", want)
		}
	}
	if t.Failed() {
		t.Log("\n" + omni)
	}
	seedance := prompt("fal", "bytedance/seedance-2.0/fast/image-to-video")
	if strings.Contains(seedance, "At 0s") || !strings.Contains(seedance, "Mei says: {如果钱不需要靠银行来信任呢？}") || !strings.Contains(seedance, "Shot 3: Action 3.\nMei says: {我们试试吧。}") {
		t.Fatalf("Seedance prompt changed:\n%s", seedance)
	}
}
