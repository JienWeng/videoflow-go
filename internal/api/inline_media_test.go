package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"videoflow-go/internal/models"
)

func TestRenderJobResponsesLeaveOutInlinePictures(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("provider must not run") })
	project, err := s.database.GetActiveProjectID()
	if err != nil {
		t.Fatal(err)
	}
	picture := "data:image/png;base64," + strings.Repeat("A", 2_000_000)
	request, _ := json.Marshal(map[string]any{
		"prompt": "A fox runs", "duration": 8, "resolution": "720p", "aspect_ratio": "16:9", "audio_mode": "native",
		"render_plan_id": "plan", "render_unit": map[string]any{"key": "plan:0", "index": 0, "count": 1},
		"frame_images":     []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": picture}, "frame_type": "first_frame"}},
		"input_references": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": picture}}},
		"director_context": map[string]any{"keyframe": map[string]any{"url": picture}},
	})
	job := &models.RenderJob{ProjectID: &project, Provider: "argolink", Model: "seedance", Status: "succeeded", RequestJSON: request}
	if err := s.database.CreateRenderJob(job); err != nil {
		t.Fatal(err)
	}

	list := call(t, s, "GET", "/render-jobs", "")
	detail := call(t, s, "GET", "/render-jobs/"+job.ID, "")
	t.Logf("stored request %d bytes; list response %d bytes; detail response %d bytes", len(request), list.Body.Len(), detail.Body.Len())
	for name, body := range map[string]string{"list": list.Body.String(), "detail": detail.Body.String()} {
		if strings.Contains(body, "data:image") || strings.Contains(body, ";base64,") {
			t.Fatalf("%s response ships picture data", name)
		}
	}
	var jobs []struct {
		RequestJSON struct {
			Prompt     string `json:"prompt"`
			Duration   int    `json:"duration"`
			Resolution string `json:"resolution"`
			Aspect     string `json:"aspect_ratio"`
			Plan       string `json:"render_plan_id"`
			Unit       struct {
				Key   string `json:"key"`
				Count int    `json:"count"`
			} `json:"render_unit"`
			Frames []struct {
				Image struct {
					URL string `json:"url"`
				} `json:"image_url"`
			} `json:"frame_images"`
		} `json:"request_json"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &jobs); err != nil || len(jobs) != 1 {
		t.Fatalf("list: %v %s", err, list.Body.String())
	}
	got := jobs[0].RequestJSON
	if got.Prompt != "A fox runs" || got.Duration != 8 || got.Resolution != "720p" || got.Aspect != "16:9" || got.Plan != "plan" || got.Unit.Key != "plan:0" || got.Unit.Count != 1 {
		t.Fatalf("small fields lost: %+v", got)
	}
	if len(got.Frames) != 1 || got.Frames[0].Image.URL != "[inline image/png, 2000022 bytes, not shown]" {
		t.Fatalf("picture marker = %+v", got.Frames)
	}
	stored, _ := s.database.GetRenderJob(job.ID)
	if !strings.Contains(string(stored.RequestJSON), picture) {
		t.Fatal("stored request lost its picture")
	}
}
