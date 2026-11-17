package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"videoflow-go/internal/agents"
)

func TestChatBriefKeepsIdeaAcrossAnswersAndAsksMissingChoices(t *testing.T) {
	previous := &agents.CreativeBrief{Idea: "A stray puppy finds Mama", PresetID: "2d-picture-book", Tone: "Hopeful", Audience: "Families"}
	got := mergeCreativeBrief(previous, &agents.CreativeBrief{Language: "English", TargetDuration: 8, SceneCount: 1}, "English, eight seconds and one scene")
	if got.Idea != previous.Idea || got.PresetID != previous.PresetID || got.SceneCount != 1 {
		t.Fatal(got)
	}
	if !strings.Contains(creativeBriefQuestion(got), "platform") {
		t.Fatal(creativeBriefQuestion(got))
	}
}
func TestConfirmedBriefRejectsMissingFieldsAndChangedStyle(t *testing.T) {
	s := routerServer(t, nil)
	project, _ := s.database.GetActiveProjectID()
	b := generationBrief{Idea: "Puppy", BriefVersion: 1, BriefConfirmed: true}
	if e := s.prepareBrief(project, &b); e == nil {
		t.Fatal("incomplete confirmation accepted")
	}
	b = generationBrief{Idea: "Puppy", BriefVersion: 1, BriefConfirmed: true, PresetID: "2d-picture-book", Tone: "Hopeful", Audience: "Families", Platform: "YouTube Shorts", AspectRatio: "9:16", Resolution: "720p", Language: "English", ConversationMode: "dialogue", SceneCount: 1, TargetDuration: 8, StyleFingerprint: "changed"}
	if e := s.prepareBrief(project, &b); e == nil {
		t.Fatal("changed style accepted")
	}
	p, e := s.findPreset(b.PresetID)
	if e != nil {
		t.Fatal(e)
	}
	b.StyleFingerprint = creativeStyleFingerprint(p)
	if e = s.prepareBrief(project, &b); e != nil {
		t.Fatal(e)
	}
	for _, part := range []string{"Hopeful", "Families", "YouTube Shorts", "8", "720p"} {
		if !strings.Contains(b.prompt(), part) {
			t.Fatal("brief choice not sent to planner", part)
		}
	}
	b.BriefConfirmed = false
	if e = s.prepareBrief(project, &b); e == nil {
		t.Fatal("unconfirmed brief accepted")
	}
}

func TestConfirmedDraftHonorsDurationFormatAndSpeech(t *testing.T) {
	b := generationBrief{BriefVersion: 1, SceneCount: 3, TargetDuration: 8, AspectRatio: "9:16", Language: "Malay", ConversationMode: "narration"}
	draft := &agents.ScriptDraft{Scenes: []agents.SceneDraft{{Duration: 1, AspectRatio: "16:9", SpokenLines: []agents.SpokenLine{{Text: "A story", Language: "English", Visible: true}}}, {Duration: 1}, {Duration: 1}}}
	if e := applyConfirmedDraft(b, draft); e != nil {
		t.Fatal(e)
	}
	sum := 0
	for _, scene := range draft.Scenes {
		sum += scene.Duration
		if scene.Duration < 1 || scene.AspectRatio != "9:16" {
			t.Fatal(draft)
		}
	}
	if sum != 8 || draft.Scenes[0].SpokenLines[0].Language != "Malay" || draft.Scenes[0].SpokenLines[0].Visible {
		t.Fatal("confirmed choices drifted", draft)
	}
	b.ConversationMode = "silent"
	if e := applyConfirmedDraft(b, draft); e != nil || len(draft.Scenes[0].SpokenLines) != 0 {
		t.Fatal(e, draft)
	}
	draft.Scenes = draft.Scenes[:2]
	if e := applyConfirmedDraft(b, draft); e == nil {
		t.Fatal("extra/missing scenes accepted")
	}
}

func TestChatBriefFollowUpUsesEditedChoicesAndOriginalIdea(t *testing.T) {
	var sawContext bool
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
			return
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected provider call %s", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		data, _ := io.ReadAll(r.Body)
		sawContext = strings.Contains(string(data), "Pip finds Mama") && strings.Contains(string(data), "Hopeful")
		text := `{"reply":"Let's confirm","action":"generate_script","creation_brief":{"language":"Malay","target_duration":12,"scene_count":2}}`
		json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": text}}}})
	})
	body := `{"message":"Malay, twelve seconds and two scenes","creation_brief":{"idea":"Pip finds Mama","style_preset_id":"2d-picture-book","tone":"Hopeful","audience":"Families","platform":"YouTube Shorts","aspect_ratio":"9:16","resolution":"720p","language":"English","conversation_mode":"dialogue","target_duration":8,"scene_count":1}}`
	r := call(t, s, "POST", "/chat", body)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	var result struct {
		Intent struct {
			Idea  string               `json:"idea"`
			Brief agents.CreativeBrief `json:"creation_brief"`
		} `json:"intent"`
	}
	json.Unmarshal(r.Body.Bytes(), &result)
	if !sawContext || result.Intent.Idea != "Pip finds Mama" || result.Intent.Brief.Language != "Malay" || result.Intent.Brief.Tone != "Hopeful" || result.Intent.Brief.SceneCount != 2 || len(result.Intent.Brief.StyleFingerprint) != 64 {
		t.Fatal(result, r.Body.String())
	}
	jobs, _ := s.database.ListRenderJobs("")
	if len(jobs) != 0 {
		t.Fatal("chat clarification started media")
	}
}

func TestConfirmedBriefResolutionAndDirectionReachRender(t *testing.T) {
	s, scene := productionFixture(t, "auto")
	var data map[string]any
	json.Unmarshal(scene.SceneJSON, &data)
	data["generation_brief"] = generationBrief{BriefVersion: 1, BriefConfirmed: true, Idea: "Pip finds Mama", Style: "ink picture book", Tone: "Hopeful", Audience: "Families", Platform: "YouTube Shorts", Resolution: "480p", AspectRatio: "9:16", Language: "English", ConversationMode: "narration", TargetDuration: 8, SceneCount: 1}
	scene.SceneJSON = voiceJSON(data)
	if e := s.database.UpdateScene(scene); e != nil {
		t.Fatal(e)
	}
	revision, err := s.sceneInputRevision(scene)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	json.Unmarshal(scene.AssetIDsJSON, &ids)
	board, err := s.database.GetAsset(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	board.MetadataJSON = voiceJSON(map[string]any{"scene_id": scene.ID, "scene_input_revision": revision, "consistency_review": "ready"})
	if err = s.database.UpdateAsset(board); err != nil {
		t.Fatal(err)
	}
	plan, e := s.sceneProductionPlan(context.Background(), scene)
	if e != nil || plan.Resolution != "480p" {
		t.Fatal(e, plan)
	}
	_, e = s.prepareAndRenderScene(context.WithValue(context.Background(), confirmedRenderPlanKey{}, plan.ID), *scene.ProjectID, scene.ID)
	if e != nil {
		t.Fatal(e)
	}
	jobs, _ := s.database.ListRenderJobs(*scene.ProjectID)
	if len(jobs) != 1 {
		t.Fatal("unexpected paid-call plan", len(jobs))
	}
	var payload map[string]any
	json.Unmarshal(jobs[0].RequestJSON, &payload)
	if payload["resolution"] != "480p" || !strings.Contains(stringPrompt(payload["prompt"]), "Hopeful") || !strings.Contains(stringPrompt(payload["prompt"]), "YouTube Shorts") {
		t.Fatal(payload)
	}
}
