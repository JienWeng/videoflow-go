package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"videoflow-go/internal/models"
)

func TestLostResponseReplayDoesNotRepeatPaidCall(t *testing.T) {
	calls := 0
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
			return
		}
		calls++
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"reply\":\"Hello\",\"action\":\"none\"}"}}]}`))
	})
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("POST", "/chat", strings.NewReader(`{"message":"Hello"}`))
		r.Header.Set("Idempotency-Key", "same-request")
		result := httptest.NewRecorder()
		s.Router().ServeHTTP(result, r)
		if result.Code != 200 {
			t.Fatalf("%d %s", result.Code, result.Body.String())
		}
	}
	if calls != 1 {
		t.Fatalf("provider called %d times for one request", calls)
	}
	r := httptest.NewRequest("POST", "/chat", strings.NewReader(`{"message":"Different"}`))
	r.Header.Set("Idempotency-Key", "same-request")
	result := httptest.NewRecorder()
	s.Router().ServeHTTP(result, r)
	if result.Code != 409 {
		t.Fatal("changed request key accepted")
	}
}
func TestUncertainRenderCannotBeResubmitted(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("provider should not be called") })
	project, _ := s.database.GetActiveProjectID()
	_ = project
	var jobID string
	// Create through the ordinary API without starting workers, then mark its durable submission intent.
	r := call(t, s, "POST", "/render", `{"prompt":"A clip"}`)
	var body map[string]any
	json.Unmarshal(r.Body.Bytes(), &body)
	jobID, _ = body["job_id"].(string)
	job, _ := s.database.GetRenderJob(jobID)
	job.Status = "reconciliation_required"
	s.database.UpdateRenderJob(job)
	if result := call(t, s, "POST", "/render-jobs/"+jobID+"/resubmit", `{}`); result.Code != 409 {
		t.Fatalf("uncertain paid request was duplicated: %d", result.Code)
	}
}

func TestResubmitLineageReturnsTheActiveReplacement(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("provider must not run") })
	job := &models.RenderJob{Provider: "openrouter", Model: "fixture", Status: "failed", RequestJSON: []byte(`{"prompt":"A clip"}`)}
	s.database.CreateRenderJob(job)
	first := call(t, s, "POST", "/render-jobs/"+job.ID+"/resubmit", `{}`)
	second := call(t, s, "POST", "/render-jobs/"+job.ID+"/resubmit", `{}`)
	var a, b map[string]any
	json.Unmarshal(first.Body.Bytes(), &a)
	json.Unmarshal(second.Body.Bytes(), &b)
	if a["job_id"] != b["job_id"] {
		t.Fatal("retrying the original job created multiple paid replacements")
	}
}

func TestMediaSettingsExposeSelectedProvider(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"data":[]}`)) })
	s.database.SetAppSetting("default_image_provider", `"fal"`)
	s.database.SetAppSetting("default_video_provider", `"fal"`)
	r := call(t, s, "GET", "/settings/app", "")
	var body map[string]any
	json.Unmarshal(r.Body.Bytes(), &body)
	if body["default_video_provider"] != "fal" || body["video_model"] != "bytedance/seedance-2.0/fast/image-to-video" {
		t.Fatal("settings ignored media selection", r.Body.String())
	}
	r = call(t, s, "PUT", "/settings/app", `{"default_video_provider":"byteplus"}`)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	json.Unmarshal(r.Body.Bytes(), &body)
	if body["video_model"] != "dreamina-seedance-2-0-260128" {
		t.Fatal("provider changed without its model", r.Body.String())
	}
}

func TestMediaProviderSwitchRemembersModelsAndRejectsMismatch(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"data":[]}`)) })
	for _, body := range []string{`{"default_video_provider":"fal","video_model":"bytedance/seedance-2.0/fast/text-to-video"}`, `{"default_video_provider":"byteplus"}`, `{"default_video_provider":"fal"}`} {
		r := call(t, s, "PUT", "/settings/app", body)
		if r.Code != 200 {
			t.Fatal(r.Code, r.Body.String())
		}
	}
	r := call(t, s, "GET", "/settings/app", "")
	var state map[string]any
	json.Unmarshal(r.Body.Bytes(), &state)
	if state["video_model"] != "bytedance/seedance-2.0/fast/text-to-video" {
		t.Fatal("provider lost its saved model", r.Body.String())
	}
	r = call(t, s, "PUT", "/settings/app", `{"default_video_provider":"byteplus","video_model":"google/veo-3.1-lite"}`)
	if r.Code != 400 {
		t.Fatal("foreign model accepted", r.Code)
	}
	r = call(t, s, "GET", "/settings/app", "")
	json.Unmarshal(r.Body.Bytes(), &state)
	if state["default_video_provider"] != "fal" {
		t.Fatal("invalid settings partially committed")
	}
	r = call(t, s, "PUT", "/settings/app", `{"default_image_provider":"byteplus"}`)
	if r.Code != 400 {
		t.Fatal("video-only provider accepted for images")
	}
}
func TestMediaConfigurationCheckDoesNotClaimGeneration(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("media setup check reached OpenRouter") })
	s.database.SetProviderSecret("fal", "test-key", nil)
	r := call(t, s, "POST", "/settings/providers/fal/test", `{}`)
	var body map[string]any
	json.Unmarshal(r.Body.Bytes(), &body)
	if body["ok"] != true || body["generation_tested"] != false || body["credentials_verified"] != false {
		t.Fatal(r.Body.String())
	}
}
