package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"videoflow-go/internal/models"
)

func productionFixture(t *testing.T, strategy string) (*Server, *models.Scene) {
	t.Helper()
	s := videoFixture(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected paid/provider request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(500)
	})
	return s, renderableScene(t, s, nil, "Home", 1, strategy)
}

// videoFixture is a server whose scene videos go to a fake fal.ai answering with fal.
func videoFixture(t *testing.T, fal http.HandlerFunc) *Server {
	t.Helper()
	s := voiceFixture(t, fal)
	s.database.SetAppSetting("default_video_provider", `"fal"`)
	s.database.SetAppSetting("video_model", `"bytedance/seedance-2.0/fast/image-to-video"`)
	return s
}

// renderableScene adds an 8-second scene with three timed shots and a current storyboard,
// so it can be rendered without making any picture first.
func renderableScene(t *testing.T, s *Server, scriptID *string, title string, order int, strategy string) *models.Scene {
	t.Helper()
	project, _ := s.database.GetActiveProjectID()
	scene := &models.Scene{ProjectID: &project, ScriptID: scriptID, SceneOrder: order, Title: title, Summary: "Pip finds Mama", Duration: 8, AspectRatio: "9:16", CharacterIDsJSON: []byte(`[]`), SceneJSON: voiceJSON(map[string]any{"production": productionSettings{AudioMode: "native", Strategy: strategy}})}
	if e := s.database.CreateScene(scene); e != nil {
		t.Fatal(e)
	}
	for i, d := range []int{3, 2, 3} {
		if e := s.database.CreateShot(&models.Shot{SceneID: scene.ID, ShotOrder: i + 1, Duration: d, Prompt: fmt.Sprintf("Action %d", i+1)}); e != nil {
			t.Fatal(e)
		}
	}
	revision, e := s.sceneInputRevision(scene)
	if e != nil {
		t.Fatal(e)
	}
	bytes, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	os.WriteFile(filepath.Join(s.cfg.StorageRoot, "board-"+scene.ID+".png"), bytes, 0600)
	board := &models.Asset{ProjectID: &project, Name: "Board", Type: "storyboard", FilePath: "storage/board-" + scene.ID + ".png", MetadataJSON: voiceJSON(map[string]any{"scene_id": scene.ID, "scene_input_revision": revision, "consistency_review": "ready"})}
	if e = s.database.CreateAsset(board); e != nil {
		t.Fatal(e)
	}
	scene, _ = s.database.GetScene(scene.ID)
	scene.AssetIDsJSON = voiceJSON([]string{board.ID})
	if e = s.database.UpdateScene(scene); e != nil {
		t.Fatal(e)
	}
	return scene
}
func TestProductionPlanBatchReuseAndChangedPlanFence(t *testing.T) {
	s, scene := productionFixture(t, "shots")
	project := *scene.ProjectID
	plan, e := s.sceneProductionPlan(context.Background(), scene)
	if e != nil {
		t.Fatal(e)
	}
	if plan.Calls != 3 || plan.NewSeconds != 12 || plan.EditedSeconds != 8 || !plan.StoryboardReady {
		t.Fatal(plan)
	}
	// Reads have no paid side effects, and a multi-call render needs its displayed plan.
	response := call(t, s, "GET", "/scenes/"+scene.ID+"/production", "")
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	response = call(t, s, "POST", "/scenes/"+scene.ID+"/render", `{"prepare_storyboard":true}`)
	if response.Code != 409 {
		t.Fatal("undisclosed batch accepted", response.Code, response.Body.String())
	}
	result, e := s.prepareAndRenderScene(context.WithValue(context.Background(), confirmedRenderPlanKey{}, plan.ID), project, scene.ID)
	if e != nil {
		t.Fatal(e)
	}
	if result.(map[string]any)["new_calls"] != 3 {
		t.Fatal(result)
	}
	jobs, _ := s.database.ListRenderJobs(project)
	if len(jobs) != 3 {
		t.Fatal(len(jobs))
	}
	for i := range jobs {
		var p map[string]any
		json.Unmarshal(jobs[i].RequestJSON, &p)
		if p["generate_audio"] != true || p["render_plan_id"] != plan.ID {
			t.Fatal(p)
		}
	}
	result, e = s.prepareAndRenderScene(context.Background(), project, scene.ID, true)
	if e != nil {
		t.Fatal(e)
	}
	if result.(map[string]any)["new_calls"] != 0 {
		t.Fatal("active jobs duplicated", result)
	}
	jobs, _ = s.database.ListRenderJobs(project)
	jobs[0].Status = "succeeded"
	jobs[1].Status = "failed"
	jobs[2].Status = "succeeded"
	for i := range jobs {
		s.database.UpdateRenderJob(&jobs[i])
	}
	result, e = s.prepareAndRenderScene(context.Background(), project, scene.ID)
	if e != nil {
		t.Fatal(e)
	}
	if result.(map[string]any)["new_calls"] != 1 || result.(map[string]any)["reused_count"] != 2 {
		t.Fatal("partial failure rerendered completed clips", result)
	}
	jobs, _ = s.database.ListRenderJobs(project)
	if len(jobs) != 4 {
		t.Fatal(len(jobs))
	}
	// Changing sound settings leaves the visual board valid but invalidates paid-call consent.
	before, _ := s.sceneInputRevision(scene)
	var doc map[string]any
	json.Unmarshal(scene.SceneJSON, &doc)
	doc["production"] = productionSettings{AudioMode: "silent", Strategy: "shots"}
	scene.SceneJSON = voiceJSON(doc)
	s.database.UpdateScene(scene)
	after, _ := s.sceneInputRevision(scene)
	if before != after {
		t.Fatal("audio choice invalidated visual storyboard")
	}
	response = call(t, s, "POST", "/scenes/"+scene.ID+"/render", fmt.Sprintf(`{"prepare_storyboard":true,"confirmed_plan_id":%q}`, plan.ID))
	if response.Code != 409 {
		t.Fatal("changed plan accepted", response.Code)
	}
	if _, e = s.prepareAndRenderScene(context.WithValue(context.Background(), confirmedRenderPlanKey{}, plan.ID), project, scene.ID); e == nil {
		t.Fatal("async stale plan accepted")
	}
}
func TestProductionPlanSettingsCASAndOwnership(t *testing.T) {
	s, scene := productionFixture(t, "auto")
	payload := fmt.Sprintf(`{"updated_at":%q,"settings":{"audio_mode":"silent","strategy":"auto","music_prompt":""}}`, scene.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"))
	r := call(t, s, "PUT", "/scenes/"+scene.ID+"/production", payload)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	if r = call(t, s, "PUT", "/scenes/"+scene.ID+"/production", payload); r.Code != 409 {
		t.Fatal("stale update accepted", r.Code)
	}
	other, _ := s.database.CreateProject("Other", "")
	s.database.ActivateProject(other.ID)
	for _, path := range []string{"production", "composition-draft"} {
		if r = call(t, s, "GET", "/scenes/"+scene.ID+"/"+path, ""); r.Code != 404 {
			t.Fatal("foreign scene exposed", path, r.Code)
		}
	}
}
func TestProductionCompositionUsesUnitOrderAndTrimsPadding(t *testing.T) {
	if _, e := exec.LookPath("ffmpeg"); e != nil {
		t.Skip("ffmpeg required for local-media assembly test")
	}
	s, scene := productionFixture(t, "shots")
	project := *scene.ProjectID
	if _, e := s.prepareAndRenderScene(context.Background(), project, scene.ID); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(s.cfg.StorageRoot, "fixture.mp4")
	cmd := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=90x160:r=24", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "4", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-y", path)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	jobs, _ := s.database.ListRenderJobs(project)
	for i := range jobs {
		jobs[i].Status = "succeeded"
		s.database.UpdateRenderJob(&jobs[i])
		if e := s.database.CreateRenderOutput(&models.RenderOutput{RenderJobID: jobs[i].ID, VideoPath: "storage/fixture.mp4"}); e != nil {
			t.Fatal(e)
		}
	}
	draft, e := s.sceneCompositionDraft(context.Background(), scene)
	if e != nil {
		t.Fatal(e)
	}
	if draft.Duration != 8 || draft.Width != 720 || draft.Height != 1280 || len(draft.Clips) != 3 {
		t.Fatal(draft)
	}
	for i, want := range []float64{3, 2, 3} {
		if draft.Clips[i].Duration != want || !draft.Clips[i].HasAudio {
			t.Fatal("padding or audio lost", draft.Clips)
		}
	}
	if draft.Clips[1].Start != 3 || draft.Clips[2].Start != 5 {
		t.Fatal("wrong clip order", draft.Clips)
	}
	jobs[0].Status = "failed"
	s.database.UpdateRenderJob(&jobs[0])
	if _, e = s.sceneCompositionDraft(context.Background(), scene); e == nil {
		t.Fatal("incomplete plan assembled")
	}
}

func TestMultiClipNativeDialogueAllocationAndUncertainFence(t *testing.T) {
	s, scene := productionFixture(t, "shots")
	var doc map[string]any
	json.Unmarshal(scene.SceneJSON, &doc)
	speaker := voiceCharacter(t, s)
	doc["spoken_lines"] = []SpokenLine{{ID: "hello", CharacterID: speaker, Text: "Hello Mama", Language: "en"}}
	scene.SceneJSON = voiceJSON(doc)
	s.database.UpdateScene(scene)
	plan, e := s.sceneProductionPlan(context.Background(), scene)
	if e != nil || plan.Blocked == "" {
		t.Fatal("unallocated dialogue repeated across clips", plan, e)
	}
	shots, _ := s.database.ListShots(scene.ID)
	doc["production"] = productionSettings{AudioMode: "native", Strategy: "shots", DialogueShots: map[string]string{"hello": shots[1].ID}}
	scene.SceneJSON = voiceJSON(doc)
	s.database.UpdateScene(scene)
	plan, e = s.sceneProductionPlan(context.Background(), scene)
	if e != nil || plan.Blocked != "" {
		t.Fatal(plan, e)
	}
	for i, unit := range plan.Units {
		job := sceneRenderJob(*scene.ProjectID, scene)
		job.RequestJSON = voiceJSON(map[string]any{"prompt": "Shot", "audio_mode": "native", "render_unit": unit})
		if e = s.applyDirectorVoicePolicy(context.Background(), job); e != nil {
			t.Fatal(e)
		}
		var payload map[string]any
		json.Unmarshal(job.RequestJSON, &payload)
		contains := strings.Contains(payload["prompt"].(string), "Hello Mama")
		if contains != (i == 1) {
			t.Fatal("dialogue assigned to wrong clip", i, payload["prompt"])
		}
	}
	job := sceneRenderJob(*scene.ProjectID, scene)
	job.RequestJSON = voiceJSON(map[string]any{"render_unit": plan.Units[0]})
	job.Status = "reconciliation_required"
	s.database.CreateRenderJob(job)
	plan, e = s.sceneProductionPlan(context.Background(), scene)
	if e != nil || plan.Blocked == "" {
		t.Fatal("uncertain paid outcome not fenced", e)
	}
}

func TestRenderSpendCannotIncreaseAfterDisplayedQuote(t *testing.T) {
	s, scene := productionFixture(t, "auto")
	project := *scene.ProjectID
	plan, e := s.sceneProductionPlan(context.Background(), scene)
	if e != nil {
		t.Fatal(e)
	}
	// A reused active clip can finish between display and an explicit new-version click.
	limit := productionSpendLimit{Calls: 0, Seconds: 0}
	ctx := context.WithValue(context.Background(), confirmedRenderSpendKey{}, limit)
	if _, e = s.prepareAndRenderScene(ctx, project, scene.ID, true); e == nil {
		t.Fatal("undisclosed new paid call accepted")
	}
	r := call(t, s, "POST", "/scenes/"+scene.ID+"/render", fmt.Sprintf(`{"prepare_storyboard":true,"confirmed_plan_id":%q,"confirmed_calls":0,"confirmed_seconds":0,"new_version":true}`, plan.ID))
	if r.Code != 409 {
		t.Fatal(r.Code, r.Body.String())
	}
	jobs, _ := s.database.ListRenderJobs(project)
	if len(jobs) != 0 {
		t.Fatal("quote mismatch created a paid job")
	}
}

func TestStoryboardSpendCannotAppearAfterDisplayedQuote(t *testing.T) {
	s, scene := productionFixture(t, "auto")
	plan, e := s.sceneProductionPlan(context.Background(), scene)
	if e != nil {
		t.Fatal(e)
	}
	var ids []string
	json.Unmarshal(scene.AssetIDsJSON, &ids)
	board, _ := s.database.GetAsset(ids[0])
	var meta map[string]any
	json.Unmarshal(board.MetadataJSON, &meta)
	meta["consistency_review"] = "rejected"
	board.MetadataJSON = voiceJSON(meta)
	s.database.UpdateAsset(board)
	r := call(t, s, "POST", "/scenes/"+scene.ID+"/render", fmt.Sprintf(`{"prepare_storyboard":true,"confirmed_plan_id":%q,"confirmed_calls":1,"confirmed_seconds":8,"confirmed_storyboard_calls":0}`, plan.ID))
	if r.Code != 409 {
		t.Fatal("unexpected image generation was not fenced", r.Code, r.Body.String())
	}
	jobs, _ := s.database.ListRenderJobs(*scene.ProjectID)
	if len(jobs) != 0 {
		t.Fatal("quote mismatch queued video")
	}
}

func TestConfirmedMultiClipHTTPPlanRunsAndReuses(t *testing.T) {
	s, scene := productionFixture(t, "shots")
	plan, e := s.sceneProductionPlan(context.Background(), scene)
	if e != nil {
		t.Fatal(e)
	}
	body := fmt.Sprintf(`{"prepare_storyboard":true,"confirmed_plan_id":%q,"confirmed_calls":3,"confirmed_seconds":12,"confirmed_storyboard_calls":0}`, plan.ID)
	for attempt := 0; attempt < 2; attempt++ {
		r := call(t, s, "POST", "/scenes/"+scene.ID+"/render", body)
		if r.Code != 200 {
			t.Fatal("confirmed UI contract rejected", r.Code, r.Body.String())
		}
		var result struct {
			NewCalls int  `json:"new_calls"`
			Reused   bool `json:"reused"`
		}
		json.Unmarshal(r.Body.Bytes(), &result)
		if attempt == 0 && result.NewCalls != 3 {
			t.Fatal(r.Body.String())
		}
		if attempt == 1 && (!result.Reused || result.NewCalls != 0) {
			t.Fatal("repeat request spent again", r.Body.String())
		}
	}
	jobs, _ := s.database.ListRenderJobs(*scene.ProjectID)
	if len(jobs) != 3 {
		t.Fatal("duplicate HTTP batch", len(jobs))
	}
}
