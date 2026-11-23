package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// A minimal MP4 header: what Go's sniffer reads as video/mp4.
var geminiMP4 = []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom")

type geminiCall struct {
	method, path, query, key, auth string
	body                           map[string]any
}

// geminiVideoServer answers each request with the next of answers (status, body) and
// records what it was sent. The handler for a path may be overridden by routes.
func geminiVideoServer(t *testing.T, answers [][2]any, routes map[string]http.HandlerFunc) (*GeminiImages, func() []geminiCall, *httptest.Server) {
	t.Helper()
	var mu sync.Mutex
	calls := []geminiCall{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		call := geminiCall{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, key: r.Header.Get("x-goog-api-key"), auth: r.Header.Get("Authorization")}
		if data, _ := io.ReadAll(r.Body); len(data) > 0 {
			if err := json.Unmarshal(data, &call.body); err != nil {
				t.Errorf("request is not JSON: %v", err)
			}
		}
		calls = append(calls, call)
		if handler := routes[r.URL.Path]; handler != nil {
			handler(w, r)
			return
		}
		if len(answers) == 0 {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			return
		}
		answer := answers[0]
		answers = answers[1:]
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(answer[0].(int))
		_, _ = w.Write([]byte(answer[1].(string)))
	}))
	t.Cleanup(server.Close)
	client := NewGeminiImages("fixture-key", server.URL+"/v1beta", GeminiVideoModel)
	client.http.client = server.Client()
	return client, func() []geminiCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]geminiCall{}, calls...)
	}, server
}

func finishedInteraction(content string) string {
	return `{"id":"v1_abc","status":"completed","steps":[{"type":"user_input"},{"type":"model_output","content":[` + content + `]}]}`
}

func TestGeminiTextToVideoStartsPollsAndDownloadsOnce(t *testing.T) {
	video := base64.StdEncoding.EncodeToString(geminiMP4)
	client, calls, _ := geminiVideoServer(t, [][2]any{
		{200, `{"id":"v1_abc","status":"in_progress","object":"interaction"}`},
		{200, `{"id":"v1_abc","status":"in_progress"}`},
		{200, finishedInteraction(`{"type":"video","mime_type":"video/mp4","data":"` + video + `"}`)},
	}, nil)
	ctx := context.Background()
	id, err := client.CreateVideo(ctx, map[string]any{"model": GeminiVideoModel, "prompt": "A kite over Penang", "duration": 8, "aspect_ratio": "9:16", "resolution": "1080p", "generate_audio": true})
	if err != nil || id != "v1_abc" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if status, _, err := client.GetVideo(ctx, id); status != "running" || err != nil {
		t.Fatalf("in progress = %q %v", status, err)
	}
	status, links, err := client.GetVideo(ctx, id)
	if status != "completed" || err != nil || len(links) != 1 || strings.Contains(links[0], "key") {
		t.Fatalf("finished = %q %v %v", status, links, err)
	}
	data, err := client.VideoContent(ctx, id, 0)
	if err != nil || string(data) != string(geminiMP4) {
		t.Fatalf("content = %q %v", data, err)
	}
	got := calls()
	if len(got) != 3 {
		t.Fatalf("the finished clip must be reused, not fetched again: %d calls", len(got))
	}
	create := got[0]
	if create.method != "POST" || create.path != "/v1beta/interactions" || create.key != "fixture-key" || create.auth != "" || create.query != "" {
		t.Fatalf("create = %+v", create)
	}
	if create.body["model"] != GeminiVideoModel || create.body["input"] != "A kite over Penang" || create.body["background"] != true || create.body["store"] != true {
		t.Fatalf("create body = %v", create.body)
	}
	if _, ok := create.body["generation_config"]; ok {
		t.Fatalf("text-to-video sends no task: %v", create.body)
	}
	format, _ := json.Marshal(create.body["response_format"])
	if string(format) != `{"aspect_ratio":"9:16","duration":"8s","resolution":"1080p","type":"video"}` {
		t.Fatalf("response_format = %s", format)
	}
	for _, field := range []string{"generate_audio", "seed", "duration", "aspect_ratio"} {
		if _, ok := create.body[field]; ok {
			t.Errorf("%s sent at the top level", field)
		}
	}
	if got[1].method != "GET" || got[1].path != "/v1beta/interactions/v1_abc" || got[1].key != "fixture-key" || got[1].query != "" {
		t.Fatalf("poll = %+v", got[1])
	}
}

func TestGeminiFirstAndLastFrameAreSentAsImagesBeforeThePrompt(t *testing.T) {
	client, calls, _ := geminiVideoServer(t, [][2]any{{200, `{"id":"v1_frames","status":"in_progress"}`}}, nil)
	frame := func(kind string) map[string]any {
		return map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64," + geminiPNG}, "frame_type": kind}
	}
	_, err := client.CreateVideo(context.Background(), map[string]any{"model": GeminiVideoModel, "prompt": "Turn to camera", "duration": 6, "frame_images": []map[string]any{frame("first_frame"), frame("last_frame")}})
	if err != nil {
		t.Fatal(err)
	}
	body := calls()[0].body
	input := body["input"].([]any)
	if len(input) != 3 || input[2].(map[string]any)["text"] != "Turn to camera" {
		t.Fatalf("input = %v", input)
	}
	for _, part := range input[:2] {
		image := part.(map[string]any)
		if image["type"] != "image" || image["mime_type"] != "image/png" || image["data"] != geminiPNG {
			t.Fatalf("frame part = %v", image)
		}
	}
	task, _ := json.Marshal(body["generation_config"])
	if string(task) != `{"video_config":{"task":"image_to_video"}}` {
		t.Fatalf("generation_config = %s", task)
	}
}

func TestGeminiReferencePicturesUseTheReferenceTask(t *testing.T) {
	client, calls, _ := geminiVideoServer(t, [][2]any{{200, `{"id":"v1_refs","status":"in_progress"}`}}, nil)
	ref := "data:image/png;base64," + geminiPNG
	if _, err := client.CreateVideo(context.Background(), map[string]any{"model": GeminiVideoModel, "prompt": "@Image 1 waves", "image_urls": []any{ref, ref}}); err != nil {
		t.Fatal(err)
	}
	body := calls()[0].body
	if len(body["input"].([]any)) != 3 {
		t.Fatalf("input = %v", body["input"])
	}
	if task, _ := json.Marshal(body["generation_config"]); string(task) != `{"video_config":{"task":"reference_to_video"}}` {
		t.Fatalf("generation_config = %s", task)
	}
}

// Everything Omni cannot do is refused before anything is sent or charged.
func TestGeminiVideoRejectsWhatOmniCannotMakeBeforeSending(t *testing.T) {
	client, calls, _ := geminiVideoServer(t, nil, nil)
	png := "data:image/png;base64," + geminiPNG
	frame := func(kind, url string) map[string]any {
		return map[string]any{"type": "image_url", "image_url": map[string]string{"url": url}, "frame_type": kind}
	}
	refs := func(n int) []any {
		out := []any{}
		for range n {
			out = append(out, png)
		}
		return out
	}
	bad := []map[string]any{
		{"model": "veo-3.1-generate-preview", "prompt": "x"},
		{"model": GeminiVideoModel, "prompt": " "},
		{"model": GeminiVideoModel, "prompt": "x", "aspect_ratio": "1:1"},
		{"model": GeminiVideoModel, "prompt": "x", "resolution": "480p"},
		{"model": GeminiVideoModel, "prompt": "x", "duration": 2},
		{"model": GeminiVideoModel, "prompt": "x", "duration": 16},
		{"model": GeminiVideoModel, "prompt": "x", "seed": 7},
		{"model": GeminiVideoModel, "prompt": "x", "audio_urls": []any{"data:audio/mpeg;base64,AA=="}},
		{"model": GeminiVideoModel, "prompt": "x", "input_references": []any{}},
		{"model": GeminiVideoModel, "prompt": "x", "image_urls": refs(7)},
		{"model": GeminiVideoModel, "prompt": "x", "image_urls": refs(1), "frame_images": []any{frame("first_frame", png)}},
		{"model": GeminiVideoModel, "prompt": "x", "frame_images": []any{frame("last_frame", png)}},
		{"model": GeminiVideoModel, "prompt": "x", "duration": 12, "frame_images": []any{frame("first_frame", png), frame("last_frame", png)}},
		{"model": GeminiVideoModel, "prompt": "x", "frame_images": []any{frame("first_frame", "http://example.com/a.png")}},
		{"model": GeminiVideoModel, "prompt": "x", "frame_images": []any{frame("first_frame", "data:image/png;base64,"+base64.StdEncoding.EncodeToString([]byte("not a picture")))}},
	}
	for _, payload := range bad {
		if err := client.ValidateVideo(context.Background(), payload); err == nil {
			t.Errorf("accepted %v", payload)
		}
		if _, err := client.CreateVideo(context.Background(), payload); err == nil {
			t.Errorf("created %v", payload)
		}
	}
	good := []map[string]any{
		{"model": GeminiVideoModel, "prompt": "x", "duration": 15, "aspect_ratio": "16:9", "resolution": "4k", "generate_audio": false},
		{"model": GeminiVideoModel, "prompt": "x", "duration": 3, "resolution": "360p"},
		{"model": GeminiVideoModel, "prompt": "x", "image_urls": refs(6)},
		{"model": GeminiVideoModel, "prompt": "x", "frame_images": []any{frame("first_frame", "https://example.com/a.png")}},
	}
	for _, payload := range good {
		if err := client.ValidateVideo(context.Background(), payload); err != nil {
			t.Errorf("rejected %v: %v", payload, err)
		}
	}
	if n := len(calls()); n != 0 {
		t.Fatalf("%d requests reached Google", n)
	}
}

func TestGeminiVideoFailuresSayWhy(t *testing.T) {
	client, _, _ := geminiVideoServer(t, [][2]any{
		{200, `{"id":"v1_abc","status":"failed","errors":[{"code":"invalid_argument","message":"Prompt blocked for fixture-key"}]}`},
		{200, finishedInteraction(`{"type":"text","text":"I can't make that video."}`)},
		{200, `{"id":"v1_abc","status":"cancelled"}`},
		{200, `{"id":"v1_abc","status":"somewhere"}`},
	}, nil)
	ctx := context.Background()
	status, _, err := client.GetVideo(ctx, "v1_abc")
	if status != "failed" || err == nil || !strings.Contains(err.Error(), "Prompt blocked") || strings.Contains(err.Error(), "fixture-key") {
		t.Fatalf("failed = %q %v", status, err)
	}
	if status, _, err = client.GetVideo(ctx, "v1_abc"); status != "failed" || err == nil || !strings.Contains(err.Error(), "can't make that video") {
		t.Fatalf("no video = %q %v", status, err)
	}
	if status, _, err = client.GetVideo(ctx, "v1_abc"); status != "failed" || err == nil {
		t.Fatalf("cancelled = %q %v", status, err)
	}
	if status, _, err = client.GetVideo(ctx, "v1_abc"); status != "" || err == nil {
		t.Fatalf("unknown status = %q %v", status, err)
	}
	if _, _, err = client.GetVideo(ctx, "../files/x"); err == nil {
		t.Fatal("unsafe interaction ID accepted")
	}
}

func TestGeminiVideoCreateErrorsKeepGooglesReason(t *testing.T) {
	client, _, _ := geminiVideoServer(t, [][2]any{
		{400, `{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.","status":"INVALID_ARGUMENT"}}`},
		{200, `{"id":"v1/../x","status":"in_progress"}`},
	}, nil)
	_, err := client.CreateVideo(context.Background(), map[string]any{"model": GeminiVideoModel, "prompt": "x"})
	var httpErr *HTTPError
	if !asHTTPError(err, &httpErr) || httpErr.Status != 400 || httpErr.Message != "INVALID_ARGUMENT API key not valid. Please pass a valid API key." || !InvalidKey(err) {
		t.Fatalf("err = %v", err)
	}
	if _, err = client.CreateVideo(context.Background(), map[string]any{"model": GeminiVideoModel, "prompt": "x"}); err == nil {
		t.Fatal("unsafe interaction ID accepted")
	}
}

func asHTTPError(err error, target **HTTPError) bool {
	e, ok := err.(*HTTPError)
	if ok {
		*target = e
	}
	return ok
}

// A clip kept in the Files API is downloaded from Google's own host, key in the header.
func TestGeminiVideoDownloadsAFileURIWithTheKeyInTheHeader(t *testing.T) {
	var server *httptest.Server
	routes := map[string]http.HandlerFunc{"/v1beta/files/abc:download": func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(geminiMP4)
	}}
	client, calls, server := geminiVideoServer(t, nil, routes)
	routes["/v1beta/interactions/v1_abc"] = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(finishedInteraction(`{"type":"video","mime_type":"video/mp4","uri":"` + server.URL + `/v1beta/files/abc:download?alt=media"}`)))
	}
	data, err := client.VideoContent(context.Background(), "v1_abc", 0)
	if err != nil || string(data) != string(geminiMP4) {
		t.Fatalf("content = %q %v", data, err)
	}
	got := calls()
	if len(got) != 2 || got[1].path != "/v1beta/files/abc:download" || got[1].query != "alt=media" || got[1].key != "fixture-key" {
		t.Fatalf("download = %+v", got)
	}
	routes["/v1beta/interactions/v1_abc"] = func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(finishedInteraction(`{"type":"video","uri":"https://elsewhere.example/v.mp4"}`)))
	}
	if _, err = client.VideoContent(context.Background(), "v1_abc", 0); err == nil {
		t.Fatal("the key would have been sent to another host")
	}
}
