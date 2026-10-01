package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testClient(t *testing.T, handler http.HandlerFunc) *OpenRouterClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewOpenRouterClientAt("test-key", srv.URL)
}

func TestVideoContract(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing auth")
		}
		switch r.URL.Path {
		case "/videos/job":
			w.Write([]byte(`{"status":"completed","unsigned_urls":["/videos/job/content?index=0"]}`))
		case "/videos/job/content":
			w.Write([]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	status, urls, err := c.GetVideo(context.Background(), "job")
	if err != nil || status != "completed" || len(urls) != 1 {
		t.Fatalf("poll contract: %s %v %v", status, urls, err)
	}
	data, err := c.VideoContent(context.Background(), "job", 0)
	if err != nil || string(data) != "\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom" {
		t.Fatalf("download: %q %v", data, err)
	}
}

func TestPollHTTPError(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"invalid key"}}`))
	})
	if _, _, err := c.GetVideo(context.Background(), "job"); err == nil {
		t.Fatal("HTTP 401 must fail")
	}
}

func TestImageContractPreservesOutputs(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/images/models" {
			w.Write([]byte(`{"data":[{"id":"test/image","supported_parameters":{"aspect_ratio":{"type":"enum","values":["16:9"]}}}]}`))
			return
		}
		if r.URL.Path != "/images" {
			t.Fatal(r.URL.Path)
		}
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["model"] != "test/image" || payload["prompt"] != "draw" {
			t.Error(payload)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString([]byte("one")), "media_type": "image/webp"}, map[string]any{"b64_json": base64.StdEncoding.EncodeToString([]byte("two")), "media_type": "image/jpeg"}}})
	})
	images, err := c.GenerateImages(context.Background(), map[string]any{"model": "test/image", "prompt": "draw", "aspect_ratio": "16:9"})
	if err != nil || len(images) != 2 {
		t.Fatalf("images: %v %v", images, err)
	}
	if images[0].MediaType != "image/webp" || string(images[1].Data) != "two" {
		t.Error(images)
	}
}

func TestVideoCapabilitiesRejectBeforeSubmission(t *testing.T) {
	posts := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			w.Write([]byte(`{"id":"job"}`))
			return
		}
		w.Write([]byte(`{"data":[{"id":"test/video","supported_durations":[4,8],"supported_aspect_ratios":["16:9"],"supported_resolutions":["720p"],"generate_audio":false}]}`))
	})
	for _, payload := range []map[string]any{
		{"model": "test/video", "prompt": "draw", "duration": 5},
		{"model": "missing", "prompt": "draw"},
		{"model": "test/video", "prompt": "draw", "generate_audio": true},
	} {
		if _, err := c.CreateVideo(context.Background(), payload); err == nil {
			t.Errorf("expected validation error: %v", payload)
		}
	}
	if posts != 0 {
		t.Fatal("unsupported requests reached paid endpoint")
	}
}

func TestStructuredJSONErrorsAndRetries(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "invalid json"}}}})
	})
	var out map[string]any
	if err := c.StructuredJSON(context.Background(), "test/model", "system", "user", &out); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if calls != 2 {
		t.Errorf("expected bounded correction retry, got %d", calls)
	}
}

func TestCredentialRedaction(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(400); w.Write([]byte("echo test-key")) })
	_, err := c.ChatCompletion(context.Background(), "m", "s", "u")
	if err == nil || strings.Contains(err.Error(), "test-key") {
		t.Fatalf("unsafe error %v", err)
	}
}

func TestTranscriptionReturnsProviderTimestamps(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			if r.URL.Query().Get("output_modalities") != "transcription" {
				t.Error("missing STT catalog filter")
			}
			w.Write([]byte(`{"data":[{"id":"openai/whisper-1"}]}`))
			return
		}
		var p map[string]any
		json.NewDecoder(r.Body).Decode(&p)
		if r.URL.Path != "/audio/transcriptions" || p["response_format"] != "verbose_json" {
			t.Error(p, r.URL.Path)
		}
		w.Write([]byte(`{"text":"Hello","segments":[{"start":0.2,"end":1.3,"text":"Hello"}]}`))
	})
	transcript, err := c.Transcribe(context.Background(), "openai/whisper-1", []byte("audio"), "wav", "en")
	if err != nil || len(transcript.Segments) != 1 || transcript.Segments[0].Start != 0.2 {
		t.Fatal(transcript, err)
	}
}

func TestVideoContentRejectsNonVideo(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>Proxy error</html>")) })
	if _, err := c.VideoContent(context.Background(), "job", 0); err == nil {
		t.Fatal("HTML cannot be accepted as a completed video")
	}
}
