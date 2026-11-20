package api

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"videoflow-go/internal/models"
)

// failedAttempts leaves a project with: scene one tried twice and failed twice; scene
// two failed and is now being made again; and two standalone clips that failed.
// On show: one's newest failure, two's running job and both clips.
func failedAttempts(t *testing.T, s *Server) (shown []string) {
	t.Helper()
	project, _ := s.database.GetActiveProjectID()
	scenes := storyScenes(t, s, "One", "Two")
	add := func(scene *string, status string) string {
		job := &models.RenderJob{ProjectID: &project, SceneID: scene, Provider: "argolink", Model: "seedance-2.0", Status: status, RequestJSON: []byte(`{"render_unit":{"index":0}}`)}
		if err := s.database.CreateRenderJob(job); err != nil {
			t.Fatal(err)
		}
		return job.ID
	}
	add(&scenes[0].ID, "failed")
	oneLatest := add(&scenes[0].ID, "failed")
	add(&scenes[1].ID, "failed")
	twoRunning := add(&scenes[1].ID, "running")
	return []string{oneLatest, twoRunning, add(nil, "failed"), add(nil, "failed")}
}

func listedJobs(t *testing.T, s *Server) []string {
	t.Helper()
	var jobs []struct {
		ID string `json:"id"`
	}
	if w := call(t, s, "GET", "/render-jobs", ""); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &jobs) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	ids := []string{}
	for _, job := range jobs {
		ids = append(ids, job.ID)
	}
	sort.Strings(ids)
	return ids
}

func canvasJobs(t *testing.T, s *Server) []string {
	t.Helper()
	var graph struct {
		Nodes []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"nodes"`
	}
	if w := call(t, s, "GET", "/graph", ""); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &graph) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	ids := []string{}
	for _, node := range graph.Nodes {
		if node.Type == "render_job" {
			ids = append(ids, node.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func TestNewerAttemptReplacesAScenesOldFailures(t *testing.T) {
	s := routerServer(t, nil)
	want := failedAttempts(t, s)
	sort.Strings(want)
	if got := listedJobs(t, s); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Videos list shows %v, want %v", got, want)
	}
	if got := canvasJobs(t, s); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("canvas shows %v, want %v", got, want)
	}
}

func TestAssistantClearsFailedVideosItself(t *testing.T) {
	s := routerServer(t, nil)
	answer := `{"reply":"I'll tidy that up.","action":"clear_failed_videos"}`
	_, prompt := plannerSays(s, &answer)
	shown := failedAttempts(t, s)
	running := shown[1]

	out := sendChat(t, s, "clean up the failed ones")
	if !strings.Contains(*prompt, `"failed_video_attempts":3`) {
		t.Fatalf("planner was not told 3 failed attempts are on show")
	}
	if out.Intent.Action != "clear_failed_videos" || !strings.Contains(out.Intent.Reply, "Cleared 3 failed video attempts") {
		t.Fatalf("action %q, reply %q", out.Intent.Action, out.Intent.Reply)
	}
	if got := listedJobs(t, s); len(got) != 1 || got[0] != running {
		t.Fatalf("Videos list shows %v, want only the running job %s", got, running)
	}
	if got := canvasJobs(t, s); len(got) != 1 || got[0] != running {
		t.Fatalf("canvas shows %v, want only the running job %s", got, running)
	}
	// Records are kept: the scene still knows its last try failed and can be made again.
	project, _ := s.database.GetActiveProjectID()
	if all, _ := s.database.ListRenderJobs(project); len(all) != 6 {
		t.Fatalf("%d jobs kept, want all 6", len(all))
	}

	if out = sendChat(t, s, "clean up again"); out.Intent.Action != "unknown" || !strings.Contains(out.Intent.Reply, "no failed video attempts") {
		t.Fatalf("second time: action %q, reply %q", out.Intent.Action, out.Intent.Reply)
	}
}

func TestClearFailedButtonSaysHowManyWereCleared(t *testing.T) {
	s := routerServer(t, nil)
	failedAttempts(t, s)
	var out struct {
		Cleared int    `json:"cleared"`
		Message string `json:"message"`
	}
	if w := call(t, s, "POST", "/render-jobs/clear-failed", ""); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Cleared != 3 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call(t, s, "POST", "/render-jobs/clear-failed", ""); json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Cleared != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
}
