package jobs

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"videoflow-go/internal/models"
)

func TestWorkerRefusesPendingGeneratedStoryboard(t *testing.T) {
	submitted := false
	worker, _ := workerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/videos/models" {
			w.Write([]byte(`{"data":[{"id":"test/video","supported_durations":[4],"supported_aspect_ratios":["16:9"],"supported_frame_images":["first_frame"]}]}`))
			return
		}
		if r.Method == "POST" {
			submitted = true
		}
		w.WriteHeader(500)
	})
	worker.cfg.UseLocalText = false
	project, _ := worker.database.GetActiveProjectID()
	scene := &models.Scene{ProjectID: &project, Title: "Review me", Duration: 4, AspectRatio: "16:9"}
	worker.database.CreateScene(scene)
	a := &models.Asset{ProjectID: &project, Type: "storyboard", Name: "Pending", FilePath: "storage/no-read-before-approval.png", MetadataJSON: json.RawMessage(`{"scene_id":"` + scene.ID + `","consistency_review":"pending"}`)}
	worker.database.CreateAsset(a)
	scene.AssetIDsJSON, _ = json.Marshal([]string{a.ID})
	worker.database.UpdateScene(scene)
	job := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Provider: "openrouter", Model: "test/video", Status: "pending", RequestJSON: json.RawMessage(`{"prompt":"A car","duration":4,"aspect_ratio":"16:9"}`)}
	worker.database.CreateRenderJob(job)
	e := worker.processJob(job.ID)
	if e == nil || !strings.Contains(e.Error(), "review") || submitted {
		t.Fatal("pending reference reused", e, submitted)
	}
}
