package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

// fakeFal answers as fal.ai would about request "fal-request-1". Any POST would be a new,
// paid video, so it is only counted.
type fakeFal struct {
	mode  atomic.Value // "done", "failed" or "locked"
	posts atomic.Int32
	asks  atomic.Int32
}

const lockedAnswer = `{"detail":"User is locked. Reason: Exhausted balance. Top up your balance at fal.ai/dashboard/billing."}`

func (f *fakeFal) handler(w http.ResponseWriter, r *http.Request) {
	mode, _ := f.mode.Load().(string)
	switch {
	case r.Method == "POST":
		f.posts.Add(1)
		w.Write([]byte(`{"request_id":"new-paid-request"}`))
	case r.URL.Path == "/clip.mp4":
		w.Write([]byte("fake mp4 bytes"))
	case strings.HasSuffix(r.URL.Path, "/status"):
		f.asks.Add(1)
		if mode == "gone" {
			w.WriteHeader(404)
			w.Write([]byte(`{"status":"NOT_FOUND"}`))
			return
		}
		if mode == "locked" {
			w.WriteHeader(403)
			w.Write([]byte(lockedAnswer))
			return
		}
		w.Write([]byte(`{"status":"COMPLETED"}`))
	case mode == "failed":
		w.WriteHeader(422)
		w.Write([]byte(`{"detail":"the request was rejected"}`))
	default:
		fmt.Fprintf(w, `{"video":{"url":"http://%s/clip.mp4"}}`, r.Host)
	}
}

func falServer(t *testing.T, mode string) (*Server, *fakeFal) {
	t.Helper()
	f := &fakeFal{}
	f.mode.Store(mode)
	s := videoFixture(t, f.handler)
	// Prices are read in the background by some screens; keep them as they are.
	s.database.SetAppSetting("video_prices_checked", time.Now().UTC().Format(time.RFC3339))
	return s, f
}

// uncertainJob is a scene video fal.ai accepted and then stopped answering about.
func uncertainJob(t *testing.T, s *Server, scene *models.Scene) *models.RenderJob {
	t.Helper()
	id, stage, message := "fal-request-1", "polling", "fal.ai HTTP 403: "+lockedAnswer
	job := &models.RenderJob{ProjectID: scene.ProjectID, SceneID: &scene.ID, Provider: "fal", Model: providers.FalVideoModel, ProviderEndpoint: s.cfg.FalBaseURL, ProviderJobID: &id, Status: "reconciliation_required", Stage: &stage, Error: &message, RequestJSON: []byte(`{}`)}
	if err := s.database.CreateRenderJob(job); err != nil {
		t.Fatal(err)
	}
	return job
}

func checkAgain(t *testing.T, s *Server, path string) renderCheck {
	t.Helper()
	w := call(t, s, "POST", path, "")
	var out renderCheck
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	return out
}

func jobStatus(t *testing.T, s *Server, id string) string {
	t.Helper()
	job, err := s.database.GetRenderJob(id)
	if err != nil {
		t.Fatal(err)
	}
	return job.Status
}

// plainForOwner fails when owner-facing text uses words the owner should never see.
func plainForOwner(t *testing.T, text string) {
	t.Helper()
	for _, word := range []string{"reconcil", "provider", "job id", "job_id", "HTTP"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(word)) {
			t.Errorf("owner text uses %q: %s", word, text)
		}
	}
}

func TestCheckAgainSavesAFinishedVideoWithoutPayingAgain(t *testing.T) {
	s, fal := falServer(t, "done")
	scene := renderableScene(t, s, nil, "The Quiet End", 1, "auto")
	job := uncertainJob(t, s, scene)
	s.workers.Start()
	t.Cleanup(s.workers.Stop)

	out := checkAgain(t, s, "/render-jobs/"+job.ID+"/check")
	if out.Outcome != "found" || out.Service != "fal.ai" {
		t.Fatalf("%+v", out)
	}
	plainForOwner(t, out.Message)
	deadline := time.Now().Add(10 * time.Second)
	for jobStatus(t, s, job.ID) != "succeeded" {
		if time.Now().After(deadline) {
			t.Fatalf("the finished video was not saved: %s", jobStatus(t, s, job.ID))
		}
		time.Sleep(20 * time.Millisecond)
	}
	if outputs, _ := s.database.ListOutputsForJob(job.ID); len(outputs) != 1 {
		t.Fatalf("outputs = %v", outputs)
	}
	if fal.posts.Load() != 0 {
		t.Fatal("checking again sent a new paid request")
	}
}

func TestCheckAgainOnAFailedVideoLetsTheSceneBeMadeAgain(t *testing.T) {
	s, fal := falServer(t, "failed")
	scene := renderableScene(t, s, nil, "The Quiet End", 1, "auto")
	job := uncertainJob(t, s, scene)
	if plan, _ := s.sceneProductionPlan(context.Background(), scene); plan.Blocked == "" {
		t.Fatal("an unanswered request did not block the scene")
	}

	out := checkAgain(t, s, "/render-jobs/"+job.ID+"/check")
	if out.Outcome != "failed" || jobStatus(t, s, job.ID) != "failed" {
		t.Fatalf("%+v status=%s", out, jobStatus(t, s, job.ID))
	}
	plainForOwner(t, out.Message)
	if plan, err := s.sceneProductionPlan(context.Background(), scene); err != nil || plan.Blocked != "" {
		t.Fatalf("scene still blocked: %v %q", err, plan.Blocked)
	}
	if fal.posts.Load() != 0 {
		t.Fatal("checking again sent a new paid request")
	}
}

// What fal.ai really answered for the owner's stuck scene once the lock lifted: it has no
// such request. That is a definite answer, so the scene must be free to be made again.
func TestCheckAgainWhenTheServiceHasNoSuchRequestLetsTheSceneBeMadeAgain(t *testing.T) {
	s, fal := falServer(t, "gone")
	scene := renderableScene(t, s, nil, "The Quiet End", 1, "auto")
	job := uncertainJob(t, s, scene)

	out := checkAgain(t, s, "/render-jobs/"+job.ID+"/check")
	if out.Outcome != "failed" || jobStatus(t, s, job.ID) != "failed" {
		t.Fatalf("%+v status=%s", out, jobStatus(t, s, job.ID))
	}
	plainForOwner(t, out.Message)
	if plan, err := s.sceneProductionPlan(context.Background(), scene); err != nil || plan.Blocked != "" {
		t.Fatalf("scene still blocked: %v %q", err, plan.Blocked)
	}
	if fal.posts.Load() != 0 {
		t.Fatal("checking again sent a new paid request")
	}
}

func TestCheckAgainOnALockedAccountWaitsAndSaysWhy(t *testing.T) {
	s, fal := falServer(t, "locked")
	scene := renderableScene(t, s, nil, "The Quiet End", 1, "auto")
	job := uncertainJob(t, s, scene)

	out := checkAgain(t, s, "/render-jobs/"+job.ID+"/check")
	if out.Outcome != "unknown" || jobStatus(t, s, job.ID) != "reconciliation_required" || !strings.Contains(out.Message, "fal.ai says the account has no balance left") {
		t.Fatalf("%+v status=%s", out, jobStatus(t, s, job.ID))
	}
	plainForOwner(t, out.Message)
	if fal.asks.Load() == 0 || fal.posts.Load() != 0 {
		t.Fatalf("asked %d times, %d new requests", fal.asks.Load(), fal.posts.Load())
	}

	// The empty account is now a known state on every screen that shows the service.
	if s.routes().Connected("fal") {
		t.Fatal("fal.ai still counts as usable")
	}
	var services struct {
		Services []videoService `json:"services"`
	}
	json.Unmarshal(call(t, s, "GET", "/video-services?model=seedance-2.0", "").Body.Bytes(), &services)
	listed := false
	for _, v := range services.Services {
		if v.Provider == "fal" {
			listed = true
			if v.Usable || v.Cheapest || !strings.Contains(v.Note, "no balance left") || v.KeyCheck == nil || v.KeyCheck.State != providers.KeyNoCredit {
				t.Fatalf("Costs table row: %+v", v)
			}
		}
	}
	if !listed {
		t.Fatal("fal.ai missing from the Costs table")
	}
	var setup struct {
		Video map[string]any `json:"video"`
	}
	json.Unmarshal(call(t, s, "GET", "/ai/setup", "").Body.Bytes(), &setup)
	if setup.Video["no_credit"] != true || !strings.Contains(fmt.Sprint(setup.Video["note"]), "no balance left") {
		t.Fatalf("AI setup: %v", setup.Video)
	}
	if body := call(t, s, "GET", "/settings/providers/fal", "").Body.String(); !strings.Contains(body, `"state":"no_credit"`) {
		t.Fatalf("Settings: %s", body)
	}
	if plan, _ := s.sceneProductionPlan(context.Background(), renderableScene(t, s, nil, "Another", 2, "auto")); !strings.Contains(plan.Blocked, "no balance left") {
		t.Fatalf("a new video would go to the empty account: %q", plan.Blocked)
	}

	// fal.ai has no free balance check: Check again forgets the answer and the next real
	// video decides.
	test := call(t, s, "POST", "/settings/providers/fal/test", "")
	if !strings.Contains(test.Body.String(), `"ok":true`) || !s.routes().Connected("fal") {
		t.Fatalf("Check again: %s", test.Body.String())
	}
}

func TestStopWaitingLetsTheSceneBeMadeAgain(t *testing.T) {
	s, fal := falServer(t, "locked")
	scene := renderableScene(t, s, nil, "The Quiet End", 1, "auto")
	job := uncertainJob(t, s, scene)

	out := checkAgain(t, s, "/render-jobs/"+job.ID+"/stop-waiting")
	if out.Outcome != "failed" || jobStatus(t, s, job.ID) != "failed" {
		t.Fatalf("%+v status=%s", out, jobStatus(t, s, job.ID))
	}
	plainForOwner(t, out.Message)
	if plan, err := s.sceneProductionPlan(context.Background(), scene); err != nil || plan.Blocked != "" {
		t.Fatalf("scene still blocked: %v %q", err, plan.Blocked)
	}
	progress, _ := s.projectProgress(context.Background(), *scene.ProjectID)
	for _, p := range progress.Scenes {
		if p.SceneID == scene.ID && p.Video.State != "failed" {
			t.Fatalf("scene video after Stop waiting: %+v", p.Video)
		}
	}
	// Pressing it again changes nothing and asks no one.
	if again := checkAgain(t, s, "/render-jobs/"+job.ID+"/stop-waiting"); again.Outcome != "failed" || fal.asks.Load() != 0 || fal.posts.Load() != 0 {
		t.Fatalf("%+v asks=%d posts=%d", again, fal.asks.Load(), fal.posts.Load())
	}
}

// finishPendingVideos plays the video service: once want jobs are waiting at the same
// time it finishes all of them. It reports how many were waiting together.
func finishPendingVideos(t *testing.T, s *Server, project string, want int) <-chan int {
	t.Helper()
	together := make(chan int, 1)
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			jobs, _ := s.database.ListRenderJobs(project)
			pending := []models.RenderJob{}
			for _, job := range jobs {
				if job.Status == "pending" || job.Status == "running" {
					pending = append(pending, job)
				}
			}
			if len(pending) >= want {
				for i := range pending {
					pending[i].Status = "succeeded"
					_ = s.database.UpdateRenderJob(&pending[i])
				}
				together <- len(pending)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		together <- 0
	}()
	return together
}

func fastAutopilot(t *testing.T) {
	previous := autopilotPoll
	autopilotPoll = 5 * time.Millisecond
	t.Cleanup(func() { autopilotPoll = previous })
}

func TestMakeSeveralScenesRendersExactlyThoseSideBySideWithoutJoining(t *testing.T) {
	fastAutopilot(t)
	s, _ := falServer(t, "done")
	project, _ := s.database.GetActiveProjectID()
	script := &models.Script{ProjectID: &project, Idea: "pup", Title: "Pup", Summary: "pup", DraftJSON: []byte("{}")}
	if e := s.database.CreateScript(script); e != nil {
		t.Fatal(e)
	}
	a := renderableScene(t, s, &script.ID, "Opening", 1, "auto")
	b := renderableScene(t, s, &script.ID, "Middle", 2, "auto")
	c := renderableScene(t, s, &script.ID, "The Quiet End", 3, "auto")

	// The price card covers exactly the chosen scenes.
	var estimate struct {
		Seconds int
		Scenes  []struct {
			SceneID string `json:"scene_id"`
		}
	}
	json.Unmarshal(call(t, s, "GET", "/costs/estimate?scene_ids="+a.ID+","+c.ID, "").Body.Bytes(), &estimate)
	if len(estimate.Scenes) != 2 || estimate.Scenes[0].SceneID != a.ID || estimate.Scenes[1].SceneID != c.ID || estimate.Seconds != 16 {
		t.Fatalf("estimate = %+v", estimate)
	}

	together := finishPendingVideos(t, s, project, 2)
	w := call(t, s, "POST", "/autopilot/finish", fmt.Sprintf(`{"script_id":%q,"scene_ids":[%q,%q],"videos_only":true}`, script.ID, c.ID, a.ID))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if n := <-together; n != 2 {
		t.Fatalf("%d videos were being made at the same time, want 2", n)
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["videos_only"] != true || result["video_url"] != nil || result["revision"] != nil || fmt.Sprint(result["scene_titles"]) != "[Opening The Quiet End]" {
		t.Fatalf("result = %v", result)
	}
	jobs, _ := s.database.ListRenderJobs(project)
	made := map[string]int{}
	for _, job := range jobs {
		made[*job.SceneID]++
	}
	if made[a.ID] != 1 || made[c.ID] != 1 || made[b.ID] != 0 {
		t.Fatalf("videos made per scene: %v", made)
	}

	// Finishing still needs the whole story, and a run needs something to make.
	for _, body := range []string{`{"scene_ids":["` + a.ID + `"]}`, `{"videos_only":true}`, `{"scene_ids":["not-a-scene"],"videos_only":true}`} {
		if w := call(t, s, "POST", "/autopilot/finish", body); w.Code < 400 {
			t.Fatalf("%s accepted: %d %s", body, w.Code, w.Body.String())
		}
	}
}

func TestAutopilotChecksAgainBeforeGivingUpOnAWaitingScene(t *testing.T) {
	fastAutopilot(t)
	t.Run("still no answer: stop before spending and name both choices", func(t *testing.T) {
		s, fal := falServer(t, "locked")
		project, _ := s.database.GetActiveProjectID()
		first := renderableScene(t, s, nil, "Opening", 1, "auto")
		stuck := renderableScene(t, s, nil, "The Quiet End", 2, "auto")
		job := uncertainJob(t, s, stuck)
		s.database.SetAppSetting("default_video_provider", `"fal"`)

		w := call(t, s, "POST", "/autopilot/finish?background=true", fmt.Sprintf(`{"scene_ids":[%q,%q],"videos_only":true}`, first.ID, stuck.ID))
		var started struct {
			OpID string `json:"op_id"`
		}
		json.Unmarshal(w.Body.Bytes(), &started)
		var op *models.Op
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if op, _ = s.database.GetOp(started.OpID); op != nil && op.Status != "running" {
				break
			}
		}
		if op == nil || op.Status != "failed" || op.Error == nil {
			t.Fatalf("op = %+v", op)
		}
		message := *op.Error
		for _, want := range []string{"The Quiet End", "fal.ai", "no balance left", "Check again", "Stop waiting", "might still bill"} {
			if !strings.Contains(message, want) {
				t.Errorf("stop message lacks %q: %s", want, message)
			}
		}
		if strings.Contains(message, "Videos") {
			t.Errorf("stop message sends the owner to another screen: %s", message)
		}
		plainForOwner(t, message)
		var result struct {
			Stuck renderCheck `json:"stuck"`
		}
		json.Unmarshal(op.ResultJSON, &result)
		if result.Stuck.JobID != job.ID || result.Stuck.SceneID != stuck.ID || result.Stuck.Service != "fal.ai" {
			t.Fatalf("stuck = %+v", result.Stuck)
		}
		if fal.asks.Load() == 0 {
			t.Fatal("autopilot gave up without checking again")
		}
		jobs, _ := s.database.ListRenderJobs(project)
		if len(jobs) != 1 || fal.posts.Load() != 0 {
			t.Fatalf("spent before stopping: %d jobs, %d requests", len(jobs), fal.posts.Load())
		}
	})
	t.Run("a video loses its answer while the run waits", func(t *testing.T) {
		s, fal := falServer(t, "locked")
		project, _ := s.database.GetActiveProjectID()
		scene := renderableScene(t, s, nil, "The Quiet End", 1, "auto")
		go func() { // the service takes the request, then stops answering
			for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
				jobs, _ := s.database.ListRenderJobs(project)
				if len(jobs) == 1 && jobs[0].Status == "pending" {
					id := "fal-request-1"
					jobs[0].Status, jobs[0].ProviderJobID, jobs[0].ProviderEndpoint = "reconciliation_required", &id, s.cfg.FalBaseURL
					_ = s.database.UpdateRenderJob(&jobs[0])
					return
				}
			}
		}()
		w := call(t, s, "POST", "/autopilot/finish", fmt.Sprintf(`{"scene_ids":[%q],"videos_only":true}`, scene.ID))
		if w.Code != 502 || !strings.Contains(w.Body.String(), "Stop waiting") || !strings.Contains(w.Body.String(), "no balance left") || fal.asks.Load() == 0 {
			t.Fatal(w.Code, w.Body.String(), fal.asks.Load())
		}
	})
	t.Run("the service says it failed: make the scene again", func(t *testing.T) {
		s, _ := falServer(t, "failed")
		project, _ := s.database.GetActiveProjectID()
		stuck := renderableScene(t, s, nil, "The Quiet End", 1, "auto")
		job := uncertainJob(t, s, stuck)
		together := finishPendingVideos(t, s, project, 1)
		w := call(t, s, "POST", "/autopilot/finish", fmt.Sprintf(`{"scene_ids":[%q],"videos_only":true}`, stuck.ID))
		if w.Code != 200 || <-together != 1 {
			t.Fatal(w.Code, w.Body.String())
		}
		if jobStatus(t, s, job.ID) != "failed" {
			t.Fatal("the old request was not closed")
		}
		jobs, _ := s.database.ListRenderJobs(project)
		if len(jobs) != 2 {
			t.Fatalf("jobs = %d, want the old one and a new one", len(jobs))
		}
	})
}

// A key the service refused must stop a scene before anything is sent, with the reason.
func TestSceneIsBlockedUpFrontWhenTheServiceRefusedTheKey(t *testing.T) {
	s, fal := falServer(t, "done")
	scene := renderableScene(t, s, nil, "Not to Mourn", 1, "auto")
	if plan, err := s.sceneProductionPlan(context.Background(), scene); err != nil || plan.Blocked != "" {
		t.Fatalf("a working service blocked the scene: %v %q", err, plan.Blocked)
	}
	key, _, _, _ := s.routes().Credentials("fal")
	if e := s.routes().SaveKeyCheck("fal", key, providers.RejectedKey("fal.ai")); e != nil {
		t.Fatal(e)
	}
	plan, _ := s.sceneProductionPlan(context.Background(), scene)
	if !strings.Contains(plan.Blocked, "did not accept this key") {
		t.Fatalf("blocked = %q", plan.Blocked)
	}
	plainForOwner(t, plan.Blocked)
	if fal.posts.Load() != 0 {
		t.Fatal("a request went to a service that refused the key")
	}
}

// "Finish the video" asked for twice while the first run is still going is one run:
// the second caller follows the first instead of starting a rival that collides with it.
func TestTheSameVideoRunAskedForTwiceIsOneRun(t *testing.T) {
	fastAutopilot(t)
	release := make(chan struct{})
	s := videoFixture(t, func(w http.ResponseWriter, r *http.Request) {
		<-release // the video service takes its time answering
		http.Error(w, "busy", 500)
	})
	s.database.SetAppSetting("video_prices_checked", time.Now().UTC().Format(time.RFC3339))
	s.database.SetAppSetting("default_video_provider", `"fal"`)
	scene := renderableScene(t, s, nil, "The Quiet End", 1, "auto")
	uncertainJob(t, s, scene)
	start := func() (id string, again bool) {
		t.Helper()
		w := call(t, s, "POST", "/autopilot/finish?background=true", fmt.Sprintf(`{"scene_ids":[%q],"videos_only":true}`, scene.ID))
		var out struct {
			OpID  string `json:"op_id"`
			Again bool   `json:"already_running"`
		}
		if w.Code != 202 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.OpID == "" {
			t.Fatal(w.Code, w.Body.String())
		}
		return out.OpID, out.Again
	}
	first, _ := start()
	second, again := start()
	if second != first || !again {
		t.Fatalf("second request started its own run %s (first %s)", second, first)
	}
	close(release)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if op, _ := s.database.GetOp(first); op != nil && op.Status != "running" {
			break
		}
	}
	// Once it is over, asking again is a new run.
	var third string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if third, _ = start(); third != first {
			break
		}
	}
	if third == first {
		t.Fatal("a finished run still blocks a new one")
	}
}
