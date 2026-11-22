package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"videoflow-go/internal/models"
)

func TestProjectProgressGuidesEachSceneToOneNextStep(t *testing.T) {
	s := routerServer(t, nil)
	project, _ := s.database.GetActiveProjectID()
	newScene := func(title string) string {
		w := call(t, s, "POST", "/scenes", `{"title":"`+title+`","summary":"x","duration":4,"aspect_ratio":"9:16"}`)
		var scene models.Scene
		if e := json.Unmarshal(w.Body.Bytes(), &scene); e != nil || scene.ID == "" {
			t.Fatal(w.Body.String())
		}
		return scene.ID
	}
	empty := newScene("Opening")
	failed := newScene("Middle")
	done := newScene("Ending")
	for _, id := range []string{failed, done} {
		if e := s.database.CreateShot(&models.Shot{SceneID: id, ShotOrder: 1, Duration: 4, Prompt: "wide"}); e != nil {
			t.Fatal(e)
		}
	}
	quota := "provider quota exceeded: secret-account-detail"
	for _, job := range []*models.RenderJob{
		{ProjectID: &project, SceneID: &failed, Status: "failed", Error: &quota},
		{ProjectID: &project, SceneID: &done, Status: "succeeded"},
	} {
		if e := s.database.CreateRenderJob(job); e != nil {
			t.Fatal(e)
		}
	}

	w := call(t, s, "GET", "/production/progress", "")
	var out projectProgress
	if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatal(w.Body.String())
	}
	stages := map[string]sceneProgress{}
	for _, p := range out.Scenes {
		stages[p.SceneID] = p
	}
	if stages[empty].Stage != "needs_shots" || stages[empty].Next.Action != "generate_shots" {
		t.Fatalf("empty scene: %+v", stages[empty])
	}
	if p := stages[failed]; p.Stage != "needs_attention" || !strings.Contains(p.Detail, "The video service says the account has no balance left") || p.Next.Action != "retry_render" {
		t.Fatalf("failed scene: %+v", p)
	}
	if stages[done].Stage != "video_ready" {
		t.Fatalf("finished scene: %+v", stages[done])
	}
	if out.Total != 3 || out.VideoReady != 1 || out.Next == nil || out.Next.SceneID != failed {
		t.Fatalf("summary: %+v", out)
	}
	if strings.Contains(w.Body.String(), "secret-account-detail") {
		t.Fatal("raw provider error leaked into progress")
	}
}

func TestProgressNeverRecommendsRetryWhenOutcomeUncertain(t *testing.T) {
	s := routerServer(t, nil)
	project, _ := s.database.GetActiveProjectID()
	scenes, _ := s.database.ListScenes(project)
	w := call(t, s, "POST", "/scenes", `{"title":"Uncertain","summary":"x","duration":4}`)
	var scene models.Scene
	_ = json.Unmarshal(w.Body.Bytes(), &scene)
	if e := s.database.CreateRenderJob(&models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Status: "reconciliation_required"}); e != nil {
		t.Fatal(e)
	}
	out, err := s.projectProgress(context.Background(), project)
	if err != nil || len(out.Scenes) != len(scenes)+1 {
		t.Fatal(err, out)
	}
	for _, p := range out.Scenes {
		if p.SceneID == scene.ID && (p.Next.Action == "retry_render" || !p.Blocked || out.Next == nil || out.Next.SceneID != scene.ID) {
			t.Fatalf("%+v next=%+v", p, out.Next)
		}
	}
}

func TestCompletedSceneRendersRequiresEveryPlannedUnit(t *testing.T) {
	scene := "scene"
	job := func(i int) models.RenderJob {
		raw, _ := json.Marshal(map[string]any{"render_plan_id": "plan", "render_unit": map[string]any{"key": "plan:" + strconvItoa(i), "index": i, "count": 2}})
		return models.RenderJob{SceneID: &scene, Status: "succeeded", RequestJSON: raw}
	}
	if completedSceneRenders([]models.RenderJob{job(0)})[scene] {
		t.Fatal("partial plan counted as complete")
	}
	if !completedSceneRenders([]models.RenderJob{job(0), job(1)})[scene] {
		t.Fatal("complete plan not detected")
	}
}

func strconvItoa(i int) string { return string(rune('0' + i)) }

func TestChatCardFallsBackToRecommendedScene(t *testing.T) {
	var sawProgress bool
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
			return
		}
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		raw, _ := json.Marshal(payload)
		sawProgress = strings.Contains(string(raw), "production_progress")
		content := `{"reply":"Make the storyboard next.","action":"storyboard"}`
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	})
	var ids []string
	for _, title := range []string{"First", "Second"} {
		w := call(t, s, "POST", "/scenes", `{"title":"`+title+`","summary":"x","duration":4}`)
		var scene models.Scene
		_ = json.Unmarshal(w.Body.Bytes(), &scene)
		ids = append(ids, scene.ID)
		call(t, s, "POST", "/scenes/"+scene.ID+"/shots", `{"shot_order":1,"duration":4,"prompt":"wide"}`)
	}
	result := call(t, s, "POST", "/chat", `{"message":"What next?"}`)
	var body struct {
		Intent struct {
			SceneID string `json:"scene_id"`
		}
	}
	_ = json.Unmarshal(result.Body.Bytes(), &body)
	if !sawProgress || body.Intent.SceneID != ids[0] {
		t.Fatalf("progress=%v %s", sawProgress, result.Body.String())
	}
}
