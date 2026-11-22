package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

// Clips made by the removed "generate from references" step stay in the project: they
// are listed and counted in the costs, and asking for one again is refused in plain
// words instead of sending the stored request somewhere it no longer fits.
func TestRetiredReferenceClipStillShowsAndCannotBeMadeAgain(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected provider call %s", r.URL)
		w.WriteHeader(500)
	})
	s.cfg.FalAPIKey = "fixture-key"
	s.cfg.FalBaseURL = s.cfg.OpenRouterBaseURL
	project, _ := s.database.GetActiveProjectID()
	request := `{"model":"bytedance/seedance-2.0/fast/reference-to-video","prompt":"Puppy waves","duration":8,"aspect_ratio":"9:16","resolution":"480p","generate_audio":true,"audio_mode":"native","reference_plan_id":"plan","image_urls":["data:image/png;base64,aA=="],"video_urls":["data:video/mp4;base64,aA=="],"audio_urls":["data:audio/wav;base64,aA=="]}`
	done := &models.RenderJob{ProjectID: &project, Provider: "fal", Model: providers.RetiredFalReferenceVideoModel, Status: "succeeded", RequestJSON: json.RawMessage(request)}
	failed := &models.RenderJob{ProjectID: &project, Provider: "fal", Model: providers.RetiredFalReferenceVideoModel, Status: "failed", RequestJSON: json.RawMessage(request)}
	for _, job := range []*models.RenderJob{done, failed} {
		if e := s.database.CreateRenderJob(job); e != nil {
			t.Fatal(e)
		}
	}
	output := &models.RenderOutput{RenderJobID: done.ID, VideoPath: "storage/outputs/old.mp4"}
	if e := s.database.CreateRenderOutput(output); e != nil {
		t.Fatal(e)
	}

	list := call(t, s, "GET", "/render-jobs", "")
	if list.Code != 200 || !strings.Contains(list.Body.String(), done.ID) || !strings.Contains(list.Body.String(), failed.ID) {
		t.Fatal("old clips are not listed", list.Code, list.Body.String())
	}
	one := call(t, s, "GET", "/render-jobs/"+done.ID, "")
	if one.Code != 200 || !strings.Contains(one.Body.String(), output.ID) {
		t.Fatal("old clip does not open", one.Code, one.Body.String())
	}
	costs := call(t, s, "GET", "/costs", "")
	if costs.Code != 200 || !strings.Contains(costs.Body.String(), `"fal"`) {
		t.Fatal("old clip is missing from the costs", costs.Code, costs.Body.String())
	}

	for _, path := range []string{"/render-jobs/" + done.ID + "/resubmit", "/render-jobs/" + failed.ID + "/resubmit", "/outputs/" + output.ID + "/retry"} {
		again := call(t, s, "POST", path, `{}`)
		if again.Code != 410 || !strings.Contains(again.Body.String(), "no longer available") {
			t.Fatal(path, again.Code, again.Body.String())
		}
	}
	if jobs, _ := s.database.ListRenderJobs(project); len(jobs) != 2 {
		t.Fatal("a retry was queued", len(jobs))
	}

	// A stored request that reaches the video service anyway is stopped before it is sent.
	client, _, e := s.routes().Media("fal", providers.RetiredFalReferenceVideoModel, "")
	if e != nil {
		t.Fatal(e)
	}
	var payload map[string]any
	json.Unmarshal([]byte(request), &payload)
	if _, e = client.CreateVideo(context.Background(), payload); !errors.Is(e, providers.ErrRetiredClip) {
		t.Fatal("retired request was not refused", e)
	}
}
