package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"videoflow-go/internal/costs"
)

const tinyPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

// Each case replays the answers a service documents and checks what the client sent.
func TestVideoHostsFollowEachServicesDocumentedAPI(t *testing.T) {
	type call struct {
		Method, Path, Auth string
		Body               map[string]any
	}
	cases := []struct {
		host                     string
		createPath, createAnswer string
		pollPath, pollAnswer     string
		uploads                  map[string]string // path → answer; {self} is the test server address
		check                    func(t *testing.T, create call)
		wantURL                  string
	}{
		{host: "argolink", createPath: "/videos/generations", createAnswer: `{"request_id":"job1","status":"pending"}`, pollPath: "/videos/job1", pollAnswer: `{"status":"done","video":{"url":"https://cdn.example.com/a.mp4"}}`,
			uploads: map[string]string{"/media/uploads": `{"upload_url":"{self}/put","media_url":"https://media.example.com/frame.png"}`},
			check: func(t *testing.T, c call) {
				if c.Body["model"] != "seedance-2.0" || c.Body["start_image"].(map[string]any)["url"] != "https://media.example.com/frame.png" || c.Body["duration"] != float64(5) {
					t.Fatalf("argolink body = %v", c.Body)
				}
			}, wantURL: "https://cdn.example.com/a.mp4"},
		{host: "replicate", createPath: "/models/bytedance/seedance-2.0/predictions", createAnswer: `{"id":"job1","status":"starting"}`, pollPath: "/predictions/job1", pollAnswer: `{"status":"succeeded","output":"https://cdn.example.com/r.mp4"}`,
			check: func(t *testing.T, c call) {
				input := c.Body["input"].(map[string]any)
				if !strings.HasPrefix(input["image"].(string), "data:image/png") || input["resolution"] != "720p" || input["aspect_ratio"] != "9:16" {
					t.Fatalf("replicate input = %v", input)
				}
			}, wantURL: "https://cdn.example.com/r.mp4"},
		{host: "kie", createPath: "/v1/jobs/createTask", createAnswer: `{"code":200,"msg":"success","data":{"taskId":"job1"}}`, pollPath: "/v1/jobs/recordInfo", pollAnswer: `{"code":200,"data":{"state":"success","resultJson":"{\"resultUrls\":[\"https://cdn.example.com/k.mp4\"]}"}}`,
			uploads: map[string]string{"/file-base64-upload": `{"data":{"downloadUrl":"https://files.example.com/frame.png"}}`},
			check: func(t *testing.T, c call) {
				input := c.Body["input"].(map[string]any)
				if c.Body["model"] != "bytedance/seedance-2" || input["first_frame_url"] != "https://files.example.com/frame.png" {
					t.Fatalf("kie body = %v", c.Body)
				}
			}, wantURL: "https://cdn.example.com/k.mp4"},
		{host: "wavespeed", createPath: "/bytedance/seedance-2.0/image-to-video", createAnswer: `{"data":{"id":"job1"}}`, pollPath: "/predictions/job1/result", pollAnswer: `{"data":{"status":"completed","outputs":["https://cdn.example.com/w.mp4"]}}`,
			uploads: map[string]string{"/media/upload/binary": `{"data":{"download_url":"https://files.example.com/w.png"}}`},
			check: func(t *testing.T, c call) {
				if c.Body["image"] != "https://files.example.com/w.png" || c.Body["generate_audio"] != false {
					t.Fatalf("wavespeed body = %v", c.Body)
				}
			}, wantURL: "https://cdn.example.com/w.mp4"},
		{host: "higgsfield", createPath: "/bytedance/seedance-2.0/image-to-video", createAnswer: `{"status":"queued","request_id":"job1"}`, pollPath: "/requests/job1/status", pollAnswer: `{"status":"completed","video":{"url":"https://cdn.example.com/h.mp4"}}`,
			uploads: map[string]string{"/files/generate-upload-url": `{"upload_url":"{self}/put","public_url":"https://files.example.com/h.png"}`},
			check: func(t *testing.T, c call) {
				if c.Body["image_url"] != "https://files.example.com/h.png" || c.Body["aspect_ratio"] != nil || !strings.HasPrefix(c.Auth, "Key ") {
					t.Fatalf("higgsfield body = %v auth = %q", c.Body, c.Auth)
				}
			}, wantURL: "https://cdn.example.com/h.mp4"},
		{host: "runway", createPath: "/image_to_video", createAnswer: `{"id":"job1"}`, pollPath: "/tasks/job1", pollAnswer: `{"status":"SUCCEEDED","output":["https://cdn.example.com/rw.mp4"]}`,
			check: func(t *testing.T, c call) {
				image := c.Body["promptImage"].([]any)[0].(map[string]any)
				if c.Body["model"] != "seedance2" || c.Body["ratio"] != "720:1280" || image["position"] != "first" || c.Body["audio"] != false {
					t.Fatalf("runway body = %v", c.Body)
				}
			}, wantURL: "https://cdn.example.com/rw.mp4"},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			var create call
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				if answer, ok := tc.uploads[r.URL.Path]; ok {
					_, _ = w.Write([]byte(strings.ReplaceAll(answer, "{self}", server.URL)))
					return
				}
				switch {
				case r.URL.Path == "/put" && r.Method == "PUT":
					if r.Header.Get("Authorization") != "" {
						t.Errorf("the service key must not be sent to the upload address")
					}
				case r.Method == "POST" && r.URL.Path == tc.createPath:
					create = call{Method: r.Method, Path: r.URL.Path, Auth: r.Header.Get("Authorization")}
					_ = json.Unmarshal(raw, &create.Body)
					if tc.host == "runway" && r.Header.Get("X-Runway-Version") == "" {
						t.Errorf("runway needs its version header")
					}
					_, _ = w.Write([]byte(tc.createAnswer))
				case r.Method == "GET" && r.URL.Path == tc.pollPath:
					_, _ = w.Write([]byte(tc.pollAnswer))
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			client := NewHostClient(tc.host, "secret", server.URL)
			model := costs.HostByName(tc.host).Models["seedance-2.0"]
			id, err := client.CreateVideo(context.Background(), map[string]any{"model": model, "prompt": "A cat walks", "duration": 5, "resolution": "720p", "aspect_ratio": "9:16", "generate_audio": false,
				"frame_images": []map[string]any{{"type": "image_url", "frame_type": "first_frame", "image_url": map[string]string{"url": tinyPNG}}}})
			if err != nil || id != "job1" {
				t.Fatalf("create: id=%q err=%v", id, err)
			}
			if create.Auth == "" || !strings.HasSuffix(create.Auth, "secret") {
				t.Fatalf("auth header = %q", create.Auth)
			}
			tc.check(t, create)
			status, urls, err := client.GetVideo(context.Background(), id)
			if err != nil || status != "completed" || len(urls) != 1 || urls[0] != tc.wantURL {
				t.Fatalf("poll: %s %v %v", status, urls, err)
			}
		})
	}
}

func TestVideoHostReportsRefusalsAndFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			_, _ = w.Write([]byte(`{"code":402,"msg":"Insufficient credits"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":200,"data":{"state":"fail","failMsg":"content rejected"}}`))
	}))
	defer server.Close()
	client := NewHostClient("kie", "secret", server.URL)
	if _, err := client.CreateVideo(context.Background(), map[string]any{"model": "bytedance/seedance-2", "prompt": "x", "duration": 5}); err == nil || !strings.Contains(err.Error(), "Insufficient credits") {
		t.Fatalf("a refusal inside HTTP 200 must be an error, got %v", err)
	}
	if status, _, err := client.GetVideo(context.Background(), "job1"); status != "failed" || err == nil || !strings.Contains(err.Error(), "content rejected") {
		t.Fatalf("failed job: %s %v", status, err)
	}
	// Loopling has no documented way to start from a picture, so it must refuse rather than drop the picture.
	loopling := NewHostClient("loopling", "secret", server.URL)
	_, err := loopling.CreateVideo(context.Background(), map[string]any{"model": "seedance-2.0", "prompt": "x", "duration": 5, "frame_images": []map[string]any{{"frame_type": "first_frame", "image_url": map[string]string{"url": tinyPNG}}}})
	if err == nil || !strings.Contains(err.Error(), "cannot start a clip from a picture") {
		t.Fatalf("loopling with a picture: %v", err)
	}
}
