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

// Replays the answers ArgoLink documents and checks what the client sent.
func TestArgoLinkFollowsItsDocumentedAPI(t *testing.T) {
	var body map[string]any
	auth := ""
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == "POST" && r.URL.Path == "/media/uploads":
			_, _ = w.Write([]byte(`{"upload_url":"` + server.URL + `/put","media_url":"https://media.example.com/frame.png"}`))
		case r.Method == "PUT" && r.URL.Path == "/put":
			if r.Header.Get("Authorization") != "" {
				t.Errorf("the service key must not be sent to the upload address")
			}
		case r.Method == "POST" && r.URL.Path == "/videos/generations":
			auth = r.Header.Get("Authorization")
			_ = json.Unmarshal(raw, &body)
			_, _ = w.Write([]byte(`{"request_id":"job1","status":"pending"}`))
		case r.Method == "GET" && r.URL.Path == "/videos/job1":
			_, _ = w.Write([]byte(`{"status":"done","video":{"url":"https://cdn.example.com/a.mp4"}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client := NewHostClient("argolink", "secret", server.URL)
	model := costs.HostByName("argolink").Models["seedance-2.0"]
	id, err := client.CreateVideo(context.Background(), map[string]any{"model": model, "prompt": "A cat walks", "duration": 5, "resolution": "720p", "aspect_ratio": "9:16", "generate_audio": false,
		"frame_images": []map[string]any{{"type": "image_url", "frame_type": "first_frame", "image_url": map[string]string{"url": tinyPNG}}}})
	if err != nil || id != "job1" {
		t.Fatalf("create: id=%q err=%v", id, err)
	}
	if auth != "Bearer secret" {
		t.Fatalf("auth header = %q", auth)
	}
	if body["model"] != "seedance-2.0" || body["start_image"].(map[string]any)["url"] != "https://media.example.com/frame.png" || body["duration"] != float64(5) ||
		body["resolution"] != "720p" || body["aspect_ratio"] != "9:16" || body["generate_audio"] != false {
		t.Fatalf("argolink body = %v", body)
	}
	status, urls, err := client.GetVideo(context.Background(), id)
	if err != nil || status != "completed" || len(urls) != 1 || urls[0] != "https://cdn.example.com/a.mp4" {
		t.Fatalf("poll: %s %v %v", status, urls, err)
	}
}

func TestArgoLinkReportsFailedJobs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"failed","error":{"message":"content rejected"}}`))
	}))
	defer server.Close()
	client := NewHostClient("argolink", "secret", server.URL)
	if status, _, err := client.GetVideo(context.Background(), "job1"); status != "failed" || err == nil || !strings.Contains(err.Error(), "content rejected") {
		t.Fatalf("failed job: %s %v", status, err)
	}
}

// ArgoLink takes a clip's connected pictures as Seedance reference images, never
// together with a first frame.
func TestArgoLinkSendsReferencePictures(t *testing.T) {
	var body map[string]any
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/media/uploads":
			_, _ = w.Write([]byte(`{"upload_url":"` + server.URL + `/put","media_url":"https://media.example.com/ref.png"}`))
		case "/put":
		case "/videos/generations":
			_ = json.Unmarshal(raw, &body)
			_, _ = w.Write([]byte(`{"request_id":"job1"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := NewHostClient("argolink", "secret", server.URL)
	request := func() map[string]any {
		return map[string]any{"model": "seedance-2.0", "prompt": "The man in @Image 1 walks through @Image 2", "duration": 5, "aspect_ratio": "9:16",
			"image_urls": []any{tinyPNG, "https://example.com/place.png"}}
	}
	if _, err := client.CreateVideo(context.Background(), request()); err != nil {
		t.Fatal(err)
	}
	refs, _ := body["reference_images"].([]any)
	if len(refs) != 2 || refs[0].(map[string]any)["url"] != "https://media.example.com/ref.png" || refs[1].(map[string]any)["url"] != "https://example.com/place.png" || body["start_image"] != nil || body["image_urls"] != nil {
		t.Fatalf("argolink body = %v", body)
	}
	mixed := request()
	mixed["frame_images"] = []map[string]any{{"type": "image_url", "frame_type": "first_frame", "image_url": map[string]string{"url": tinyPNG}}}
	if err := client.ValidateVideo(context.Background(), mixed); err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("references with a first frame: %v", err)
	}
	many := request()
	many["image_urls"] = []any{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	if err := client.ValidateVideo(context.Background(), many); err == nil || !strings.Contains(err.Error(), "at most 9") {
		t.Fatalf("ten references: %v", err)
	}
}

// A reference-mode clip may also carry voice recordings: uploaded like a picture and sent
// as reference_audios, never without a reference picture.
func TestArgoLinkSendsVoiceRecordings(t *testing.T) {
	var body, ticket map[string]any
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/media/uploads":
			_ = json.Unmarshal(raw, &ticket)
			_, _ = w.Write([]byte(`{"upload_url":"` + server.URL + `/put","media_url":"https://media.example.com/voice.mp3"}`))
		case "/put":
		case "/videos/generations":
			_ = json.Unmarshal(raw, &body)
			_, _ = w.Write([]byte(`{"request_id":"job1"}`))
		}
	}))
	defer server.Close()
	client := NewHostClient("argolink", "secret", server.URL)
	request := map[string]any{"model": "seedance-2.0", "prompt": "Define the person in @Image 1 as Chen Wei. Chen Wei speaks with the voice in @Audio 1.", "duration": 5,
		"image_urls": []any{"https://example.com/chen.png"}, "audio_urls": []any{"data:audio/mpeg;base64,SUQz"}}
	if _, err := client.CreateVideo(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	voices, _ := body["reference_audios"].([]any)
	if len(voices) != 1 || voices[0].(map[string]any)["url"] != "https://media.example.com/voice.mp3" || ticket["type"] != "audio" || ticket["content_type"] != "audio/mpeg" || body["audio_urls"] != nil {
		t.Fatalf("argolink body = %v ticket = %v", body, ticket)
	}
	request["audio_urls"] = []any{"a", "b", "c", "d"}
	if err := client.ValidateVideo(context.Background(), request); err == nil || !strings.Contains(err.Error(), "at most 3 voice") {
		t.Fatalf("too many voices: %v", err)
	}
	delete(request, "image_urls")
	request["audio_urls"] = []any{"a"}
	if err := client.ValidateVideo(context.Background(), request); err == nil || !strings.Contains(err.Error(), "only with reference pictures") {
		t.Fatalf("voice without a picture: %v", err)
	}
}
