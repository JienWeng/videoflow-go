package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFalVideoUsesQueueAndExactFields(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Key fal-secret" {
			t.Error("wrong fal authentication")
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /bytedance/seedance-2.0/fast/image-to-video":
			posts++
			var p map[string]any
			json.NewDecoder(r.Body).Decode(&p)
			if p["image_url"] != "data:image/png;base64,aA==" || p["duration"] != "5" || p["model"] != nil || p["frame_images"] != nil {
				t.Errorf("wrong fal payload: %+v", p)
			}
			w.Write([]byte(`{"request_id":"fal-task","status_url":"https://evil.invalid/status"}`))
		case "GET /bytedance/seedance-2.0/requests/fal-task/status":
			w.Write([]byte(`{"status":"COMPLETED"}`))
		case "GET /bytedance/seedance-2.0/requests/fal-task":
			w.Write([]byte(`{"video":{"url":"https://cdn.example/video.mp4"}}`))
		default:
			t.Error(r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := NewFalClientAt("fal-secret", server.URL, "bytedance/seedance-2.0/fast/image-to-video")
	p := map[string]any{"model": "bytedance/seedance-2.0/fast/image-to-video", "prompt": "Cat", "duration": 5, "aspect_ratio": "16:9", "frame_images": []map[string]any{{"frame_type": "first_frame", "image_url": map[string]string{"url": "data:image/png;base64,aA=="}}}}
	id, err := c.CreateVideo(context.Background(), p)
	if err != nil || id != "fal-task" {
		t.Fatal(id, err)
	}
	status, urls, err := c.GetVideo(context.Background(), id)
	if err != nil || status != "completed" || len(urls) != 1 || posts != 1 {
		t.Fatal(status, urls, err, posts)
	}
}
func TestBytePlusVideoUsesDirectTaskAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ark-secret" {
			t.Error("wrong BytePlus authentication")
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /contents/generations/tasks":
			var p map[string]any
			json.NewDecoder(r.Body).Decode(&p)
			if p["ratio"] != "9:16" || p["aspect_ratio"] != nil || p["frame_images"] != nil || p["model"] != "dreamina-seedance-2-0-260128" {
				t.Errorf("wrong BytePlus payload %+v", p)
			}
			content := p["content"].([]any)
			if len(content) != 2 || content[1].(map[string]any)["role"] != "first_frame" {
				t.Error(content)
			}
			w.Write([]byte(`{"id":"ark-task"}`))
		case "GET /contents/generations/tasks/ark-task":
			w.Write([]byte(`{"status":"succeeded","content":{"video_url":"https://cdn.example/ark.mp4"}}`))
		default:
			t.Error(r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := NewBytePlusClientAt("ark-secret", server.URL)
	p := map[string]any{"model": "dreamina-seedance-2-0-260128", "prompt": "Cat", "duration": 5, "aspect_ratio": "9:16", "frame_images": []map[string]any{{"frame_type": "first_frame", "image_url": map[string]string{"url": "data:image/png;base64,aA=="}}}}
	id, err := c.CreateVideo(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	status, urls, err := c.GetVideo(context.Background(), id)
	if err != nil || status != "completed" || len(urls) != 1 {
		t.Fatal(status, urls, err)
	}
}
func TestMediaAdaptersRejectForeignModelAndMissingFrameBeforePOST(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer s.Close()
	c := NewFalClientAt("secret", s.URL, "bytedance/seedance-2.0/fast/image-to-video")
	for _, model := range []string{"google/veo-3.1-lite", "bytedance/seedance-2.0/fast/image-to-video"} {
		if _, err := c.CreateVideo(context.Background(), map[string]any{"model": model, "prompt": "Cat", "duration": 5}); err == nil {
			t.Error("accepted unsupported request", model)
		}
	}
	if calls != 0 {
		t.Fatal("invalid request reached paid endpoint")
	}
}

func TestFalImagesUseEditEndpointAndDownloadWithoutKey(t *testing.T) {
	var base string
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /fal-ai/nano-banana-2/edit":
			posts++
			if r.Header.Get("Authorization") != "Key secret" {
				t.Error("wrong key")
			}
			var p map[string]any
			json.NewDecoder(r.Body).Decode(&p)
			refs, ok := p["image_urls"].([]any)
			if !ok || len(refs) != 1 || p["input_references"] != nil || p["model"] != nil {
				t.Error(p)
			}
			w.Write([]byte(`{"request_id":"image-task"}`))
		case "GET /fal-ai/nano-banana-2/requests/image-task/status":
			w.Write([]byte(`{"status":"COMPLETED"}`))
		case "GET /fal-ai/nano-banana-2/requests/image-task":
			json.NewEncoder(w).Encode(map[string]any{"images": []any{map[string]string{"url": base + "/image.png"}}})
		case "GET /image.png":
			if r.Header.Get("Authorization") != "" {
				t.Error("key leaked to output download")
			}
			w.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 0})
		default:
			t.Error(r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base = server.URL
	c := NewFalClientAt("secret", base, FalImageModel)
	images, err := c.GenerateImages(context.Background(), map[string]any{"model": FalImageModel, "prompt": "Mimi", "aspect_ratio": "16:9", "input_references": []map[string]any{{"image_url": map[string]string{"url": "data:image/png;base64,aA=="}}}})
	if err != nil || len(images) != 1 || posts != 1 {
		t.Fatal(images, err, posts)
	}
}
func TestFalCompletedQueueCanContainFailedResult(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/status") {
			w.Write([]byte(`{"status":"COMPLETED"}`))
			return
		}
		w.WriteHeader(422)
		w.Write([]byte(`{"detail":"secret: rejected"}`))
	}))
	defer s.Close()
	c := NewFalClientAt("secret", s.URL, FalVideoModel)
	status, _, err := c.GetVideo(context.Background(), "task")
	if status != "failed" || err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal(status, err)
	}
}

func TestPublicMediaRejectsPrivateProductionURL(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.Write([]byte("private")) }))
	defer s.Close()
	if _, err := publicMedia(context.Background(), s.URL, 1024); err == nil || calls != 0 {
		t.Fatal("production output download reached a private HTTP service", err, calls)
	}
}
