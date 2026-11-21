package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

// plannerSays makes the chat planner answer with reply and keeps what it was told.
func plannerSays(s *Server, reply *string) (system, prompt *string) {
	system, prompt = new(string), new(string)
	s.agents.SetStructuredRunner(func(_ context.Context, _, sys, user string, target any) error {
		*system, *prompt = sys, user
		return json.Unmarshal([]byte(*reply), target)
	})
	return system, prompt
}

type chatReply struct {
	Intent struct {
		Action   string   `json:"action"`
		Reply    string   `json:"reply"`
		SceneIDs []string `json:"scene_ids"`
		Scenes   []struct {
			ID, Title string
			Duration  int
		} `json:"scenes"`
		JobID      string `json:"job_id"`
		SceneID    string `json:"scene_id"`
		SceneTitle string `json:"scene_title"`
		Service    string `json:"service"`
	} `json:"intent"`
}

func sendChat(t *testing.T, s *Server, message string) chatReply {
	t.Helper()
	w := call(t, s, "POST", "/chat", `{"message":"`+message+`"}`)
	var out chatReply
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	return out
}

// storyScenes adds one story with the given scene titles, in order.
func storyScenes(t *testing.T, s *Server, titles ...string) []models.Scene {
	t.Helper()
	project, _ := s.database.GetActiveProjectID()
	script := &models.Script{ProjectID: &project, Idea: "pup", Title: "Pup", Summary: "pup", DraftJSON: []byte("{}")}
	if e := s.database.CreateScript(script); e != nil {
		t.Fatal(e)
	}
	scenes := []models.Scene{}
	for i, title := range titles {
		scene := models.Scene{ProjectID: &project, ScriptID: &script.ID, SceneOrder: i + 1, Title: title, Summary: title, Duration: 8, AspectRatio: "9:16", CharacterIDsJSON: []byte("[]"), SceneJSON: []byte("{}")}
		if e := s.database.CreateScene(&scene); e != nil {
			t.Fatal(e)
		}
		scenes = append(scenes, scene)
	}
	return scenes
}

func sceneJob(t *testing.T, s *Server, scene models.Scene, status, message string) *models.RenderJob {
	t.Helper()
	job := &models.RenderJob{ProjectID: scene.ProjectID, SceneID: &scene.ID, Provider: "fal", Model: providers.FalVideoModel, Status: status, RequestJSON: []byte("{}")}
	if message != "" {
		job.Error = &message
	}
	if status == "reconciliation_required" {
		id := "fal-request-1"
		job.ProviderJobID = &id
	}
	if e := s.database.CreateRenderJob(job); e != nil {
		t.Fatal(e)
	}
	return job
}

func TestChatMakesSeveralSceneVideosFromOneReply(t *testing.T) {
	answer := `{"reply":"I'll make both.","action":"render_scenes"}`
	s := routerServer(t, nil)
	system, _ := plannerSays(s, &answer)
	scenes := storyScenes(t, s, "Opening", "Middle", "The Quiet End")
	sceneJob(t, s, scenes[0], "succeeded", "")
	sceneJob(t, s, scenes[2], "failed", "fal.ai HTTP 500: upstream")

	// Nothing named: every scene of the episode still without a video.
	out := sendChat(t, s, "make the rest of the videos")
	if out.Intent.Action != "render_scenes" || strings.Join(out.Intent.SceneIDs, ",") != scenes[1].ID+","+scenes[2].ID {
		t.Fatalf("%+v", out.Intent)
	}
	if len(out.Intent.Scenes) != 2 || out.Intent.Scenes[0].Title != "Middle" || out.Intent.Scenes[1].Title != "The Quiet End" || out.Intent.Scenes[1].Duration != 8 {
		t.Fatalf("card scenes = %+v", out.Intent.Scenes)
	}
	if strings.Contains(*system, "one scene per reply") || !strings.Contains(*system, "render_scenes") {
		t.Fatal("the planner is still told to make one scene at a time")
	}

	// Named scenes, by title or id, in story order.
	answer = `{"reply":"On it.","action":"render_scenes","scene_ids":["the quiet end","` + scenes[0].ID + `"]}`
	out = sendChat(t, s, "redo the first and the last")
	if strings.Join(out.Intent.SceneIDs, ",") != scenes[0].ID+","+scenes[2].ID {
		t.Fatalf("%+v", out.Intent)
	}

	// Nothing left to make: no paid button.
	answer = `{"reply":"On it.","action":"render_scenes","scene_ids":["no such scene"]}`
	if out = sendChat(t, s, "make scene nine"); out.Intent.Action != "unknown" {
		t.Fatalf("%+v", out.Intent)
	}
}

func TestChatContextTellsThePlannerWhereEachSceneVideoStands(t *testing.T) {
	answer := `{"reply":"fal.ai has not answered about The Quiet End.","action":"check_video"}`
	s := routerServer(t, nil)
	_, prompt := plannerSays(s, &answer)
	s.cfg.FalAPIKey = "test"
	s.database.SetAppSetting("default_video_provider", `"fal"`)
	s.routes().NoteServiceAnswer("fal", &providers.HTTPError{Provider: "fal.ai", Status: 403, Message: lockedAnswer})
	scenes := storyScenes(t, s, "Ready", "Making", "Failed", "The Quiet End", "Untouched")
	sceneJob(t, s, scenes[0], "succeeded", "")
	sceneJob(t, s, scenes[1], "running", "")
	sceneJob(t, s, scenes[2], "failed", "provider quota exceeded: secret-account-detail")
	waiting := sceneJob(t, s, scenes[3], "reconciliation_required", "fal.ai HTTP 403: "+lockedAnswer)

	out := sendChat(t, s, "why is the last scene stuck?")
	if out.Intent.Action != "check_video" || out.Intent.JobID != waiting.ID || out.Intent.SceneID != scenes[3].ID || out.Intent.Service != "fal.ai" {
		t.Fatalf("%+v", out.Intent)
	}

	_, raw, _ := strings.Cut(*prompt, "Project context: ")
	var context struct {
		Progress struct {
			Scenes []sceneProgress `json:"scenes"`
		} `json:"production_progress"`
		Service map[string]any `json:"video_service"`
	}
	if err := json.Unmarshal([]byte(raw), &context); err != nil {
		t.Fatal(err, raw)
	}
	states := map[string]sceneVideo{}
	for _, p := range context.Progress.Scenes {
		states[p.Title] = p.Video
	}
	want := map[string]string{"Ready": "ready", "Making": "making", "Failed": "failed", "The Quiet End": "waiting_on_service", "Untouched": "none"}
	for title, state := range want {
		if states[title].State != state {
			t.Errorf("%s: video = %+v, want %s", title, states[title], state)
		}
	}
	if v := states["The Quiet End"]; v.JobID != waiting.ID || !strings.Contains(v.Reason, "no balance left") || v.Service != "fal.ai" {
		t.Errorf("waiting scene = %+v", v)
	}
	if v := states["Failed"]; !strings.Contains(v.Reason, "no balance left") || strings.Contains(v.Reason, "secret-account-detail") {
		t.Errorf("failed scene reason = %q", v.Reason)
	}
	if problem, _ := context.Service["problem"].(string); !strings.Contains(problem, "no balance left") {
		t.Errorf("video service = %v", context.Service)
	}

	// Stop waiting is offered the same way, and only for a scene that is waiting.
	answer = `{"reply":"OK.","action":"stop_waiting","scene_id":"Ready"}`
	if out = sendChat(t, s, "stop waiting on the first scene"); out.Intent.Action != "unknown" {
		t.Fatalf("%+v", out.Intent)
	}
	answer = `{"reply":"OK.","action":"stop_waiting","job_id":"` + waiting.ID + `"}`
	if out = sendChat(t, s, "forget it, make it again"); out.Intent.Action != "stop_waiting" || out.Intent.JobID != waiting.ID || out.Intent.SceneTitle != "The Quiet End" {
		t.Fatalf("%+v", out.Intent)
	}
}
