package api

import (
	"testing"
	"videoflow-go/internal/models"
)

func TestSceneRenderDoesNotSpendTwice(t *testing.T) {
	for _, status := range []string{"pending", "running", "succeeded", "reconciliation_required", "failed"} {
		t.Run(status, func(t *testing.T) {
			s := routerServer(t, nil)
			project, _ := s.database.GetActiveProjectID()
			scene := &models.Scene{ProjectID: &project, Title: "Home", Duration: 8, AspectRatio: "9:16"}
			if err := s.database.CreateScene(scene); err != nil {
				t.Fatal(err)
			}
			old := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Provider: "fal", Model: "fixture", Status: status, RequestJSON: []byte(`{"duration":8,"prompt":"Home"}`)}
			if err := s.database.CreateRenderJob(old); err != nil {
				t.Fatal(err)
			}
			candidate := *old
			candidate.ID = ""
			candidate.RequestJSON = []byte(`{"prompt":"Home","duration":8}`)
			got, err := s.existingSceneRender(&candidate, false)
			if err != nil {
				t.Fatal(err)
			}
			if (got != nil) != (status != "failed") {
				t.Fatalf("status %s reuse=%v", status, got != nil)
			}
			got, err = s.existingSceneRender(&candidate, true)
			if err != nil {
				t.Fatal(err)
			}
			if (got != nil) != (status != "failed" && status != "succeeded") {
				t.Fatal("explicit version bypassed active job or failed to allow completed version")
			}
			candidate.RequestJSON = []byte(`{"duration":8,"prompt":"Changed"}`)
			got, err = s.existingSceneRender(&candidate, false)
			active := status == "pending" || status == "running" || status == "reconciliation_required"
			if active && err == nil {
				t.Fatal("changed inputs allowed a concurrent paid render")
			}
			if !active && (err != nil || got != nil) {
				t.Fatal("changed completed inputs incorrectly reused", err)
			}
		})
	}
}

func TestSceneReuseSurvivesWorkerPayloadEnrichment(t *testing.T) {
	s := routerServer(t, nil)
	project, _ := s.database.GetActiveProjectID()
	scene := &models.Scene{ProjectID: &project, Title: "Home", Duration: 8, AspectRatio: "9:16"}
	s.database.CreateScene(scene)
	old := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Provider: "fal", Model: "fixture", Status: "succeeded", RequestJSON: []byte(`{"request_fingerprint":"stable","prompt":"Scene plus worker instructions","model":"fixture","duration":8}`)}
	s.database.CreateRenderJob(old)
	candidate := *old
	candidate.RequestJSON = []byte(`{"request_fingerprint":"stable","prompt":"Scene"}`)
	got, e := s.existingSceneRender(&candidate, false)
	if e != nil || got == nil || got.ID != old.ID {
		t.Fatal("worker enrichment caused another paid render", got, e)
	}
}
