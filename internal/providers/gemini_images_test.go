package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const geminiPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j4v8AAAAASUVORK5CYII="

// geminiServer answers every generateContent call with status and body, and hands the
// decoded request to inspect.
func geminiServer(t *testing.T, status int, body string, inspect func(*http.Request, map[string]any)) (*GeminiImages, func() int) {
	t.Helper()
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request map[string]any
		data, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(data, &request); err != nil {
			t.Errorf("request is not JSON: %v", err)
		}
		if inspect != nil {
			inspect(r, request)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	client := NewGeminiImages("fixture-key", server.URL+"/v1beta", GeminiImageModel)
	client.http.client = server.Client()
	return client, func() int { return calls }
}

func imageAnswer(parts string) string {
	return `{"candidates":[{"content":{"role":"model","parts":[` + parts + `]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":1120}}`
}

func TestGeminiImageRequestCarriesPromptReferencesAndAspect(t *testing.T) {
	client, calls := geminiServer(t, 200, imageAnswer(`{"text":"Here you go"},{"inlineData":{"mimeType":"image/png","data":"`+geminiPNG+`"}}`), func(r *http.Request, body map[string]any) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1beta/models/gemini-2.5-flash-image:generateContent" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "fixture-key" || r.Header.Get("Authorization") != "" || r.URL.RawQuery != "" {
			t.Error("key must travel only in the x-goog-api-key header")
		}
		contents := body["contents"].([]any)
		parts := contents[0].(map[string]any)["parts"].([]any)
		if len(parts) != 3 || parts[0].(map[string]any)["text"] != "A red kite over Penang" {
			t.Fatalf("want prompt then two pictures, got %v", parts)
		}
		for _, part := range parts[1:] {
			inline := part.(map[string]any)["inlineData"].(map[string]any)
			if inline["mimeType"] != "image/png" || inline["data"] != geminiPNG {
				t.Errorf("reference sent as %v", inline)
			}
		}
		config := body["generationConfig"].(map[string]any)
		modalities, _ := json.Marshal(config["responseModalities"])
		if string(modalities) != `["TEXT","IMAGE"]` {
			t.Errorf("responseModalities = %s", modalities)
		}
		if config["imageConfig"].(map[string]any)["aspectRatio"] != "9:16" {
			t.Errorf("imageConfig = %v", config["imageConfig"])
		}
	})
	ref := map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64," + geminiPNG}}
	images, err := client.GenerateImages(context.Background(), map[string]any{
		"model": "gemini-2.5-flash-image", "prompt": "A red kite over Penang", "aspect_ratio": "9:16",
		"input_references": []map[string]any{ref, ref},
	})
	if err != nil || len(images) != 1 || calls() != 1 {
		t.Fatalf("images=%d calls=%d err=%v", len(images), calls(), err)
	}
	if images[0].MediaType != "image/png" || len(images[0].Data) == 0 || !strings.Contains(string(images[0].Usage), "candidatesTokenCount") {
		t.Fatalf("unexpected image %q usage %s", images[0].MediaType, images[0].Usage)
	}
}

func TestGeminiImageWithoutAspectOmitsImageConfig(t *testing.T) {
	client, _ := geminiServer(t, 200, imageAnswer(`{"inlineData":{"mimeType":"image/png","data":"`+geminiPNG+`"}}`), func(_ *http.Request, body map[string]any) {
		if strings.Contains(toJSON(body), "imageConfig") {
			t.Error("imageConfig sent without an aspect ratio")
		}
	})
	if _, err := client.GenerateImages(context.Background(), map[string]any{"prompt": "A lantern"}); err != nil {
		t.Fatal(err)
	}
}

func toJSON(v any) string { data, _ := json.Marshal(v); return string(data) }

func TestGeminiImageSkipsThoughtDrafts(t *testing.T) {
	// A draft picture arrives as a thought before the final one; only the final is kept.
	draft := "R0lGODlhAQABAIAAAP///wAAACwAAAAAAQABAAACAkQBADs=" // a GIF
	client, _ := geminiServer(t, 200, imageAnswer(`{"thought":true,"inlineData":{"mimeType":"image/gif","data":"`+draft+`"}},{"inlineData":{"mimeType":"image/png","data":"`+geminiPNG+`"}}`), nil)
	images, err := client.GenerateImages(context.Background(), map[string]any{"prompt": "A lantern"})
	if err != nil || len(images) != 1 || images[0].MediaType != "image/png" {
		t.Fatalf("images=%v err=%v", images, err)
	}
}

func TestGeminiImageErrors(t *testing.T) {
	cases := []struct {
		name, body, want string
		status           int
	}{
		{"bad key", `{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.","status":"INVALID_ARGUMENT"}}`, "Google Gemini HTTP 400: INVALID_ARGUMENT API key not valid", 400},
		{"quota", `{"error":{"code":429,"message":"Resource has been exhausted","status":"RESOURCE_EXHAUSTED"}}`, "HTTP 429: RESOURCE_EXHAUSTED", 429},
		{"server", `upstream exploded`, "Google Gemini HTTP 503: upstream exploded", 503},
		{"prompt blocked", `{"promptFeedback":{"blockReason":"PROHIBITED_CONTENT"}}`, "refused the prompt: Google flagged the request as prohibited content (PROHIBITED_CONTENT)", 200},
		{"picture blocked", `{"candidates":[{"content":{"parts":[]},"finishReason":"IMAGE_SAFETY"}]}`, "returned no picture because the picture was blocked by Google's safety filters (IMAGE_SAFETY)", 200},
		{"text only", imageAnswer(`{"text":"I can't draw that."}`), "the model answered without a picture: I can't draw that.", 200},
		{"no candidates", `{"candidates":[]}`, "returned no picture", 200},
		{"bad data", imageAnswer(`{"inlineData":{"mimeType":"image/png","data":"not base64!"}}`), "invalid picture data", 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := geminiServer(t, tc.status, tc.body, nil)
			_, err := client.GenerateImages(context.Background(), map[string]any{"prompt": "A lantern"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "fixture-key") {
				t.Fatal("error leaks the API key")
			}
		})
	}
}

func TestGeminiImageKeyIsRedactedFromErrors(t *testing.T) {
	client, _ := geminiServer(t, 403, `{"error":{"message":"key fixture-key is suspended","status":"PERMISSION_DENIED"}}`, nil)
	_, err := client.GenerateImages(context.Background(), map[string]any{"prompt": "A lantern"})
	if err == nil || strings.Contains(err.Error(), "fixture-key") || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("error = %v", err)
	}
}

func TestGeminiImageModelsAndValidation(t *testing.T) {
	client := NewGeminiImages("", GeminiURL, GeminiImageModel)
	models, err := client.Models(context.Background(), "image")
	if err != nil || len(models) != len(GeminiImageModels) {
		t.Fatalf("models=%v err=%v", models, err)
	}
	ids := []string{}
	for _, m := range models {
		ids = append(ids, m.ID)
	}
	if strings.Join(ids, ",") != "gemini-3-pro-image,gemini-3.1-flash-image,gemini-2.5-flash-image" {
		t.Fatalf("catalog = %v", ids)
	}
	if video, _ := client.Models(context.Background(), "video"); len(video) != 1 || video[0].ID != GeminiVideoModel {
		t.Fatalf("Gemini video catalog = %v", video)
	}
	pro, err := client.Model(context.Background(), "gemini-3-pro-image", "image")
	if err != nil {
		t.Fatal(err)
	}
	var caps map[string]struct {
		Values []string `json:"values"`
		Max    int      `json:"max"`
	}
	if json.Unmarshal(pro.SupportedParameters, &caps) != nil || caps["input_references"].Max != 14 || len(caps["aspect_ratio"].Values) != 10 || contains(pro.AspectRatios, "1:8") {
		t.Fatalf("Pro capabilities = %s %v", pro.SupportedParameters, pro.AspectRatios)
	}
	nb2, _ := client.Model(context.Background(), "gemini-3.1-flash-image", "image")
	if !contains(nb2.AspectRatios, "8:1") {
		t.Fatalf("Nano Banana 2 ratios = %v", nb2.AspectRatios)
	}
	if _, err := client.Model(context.Background(), "gemini-3-pro-image-preview", "image"); err == nil {
		t.Fatal("the shut-down preview id must not be offered")
	}
	if _, err := client.Model(context.Background(), GeminiImageModel, "video"); err == nil {
		t.Fatal("Gemini image model accepted for video")
	}

	ref := map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64," + geminiPNG}}
	refs := func(n int) []map[string]any {
		out := []map[string]any{}
		for range n {
			out = append(out, ref)
		}
		return out
	}
	bad := []map[string]any{
		{"model": "gemini-2.5-flash-image", "prompt": "x", "input_references": refs(4)},
		{"model": "gemini-3-pro-image", "prompt": "x", "input_references": refs(15)},
		{"model": "gemini-3-pro-image", "prompt": "x", "aspect_ratio": "8:1"},
		{"model": "gemini-3-pro-image", "prompt": " "},
		{"model": "gemini-3-pro-image", "prompt": "x", "seed": 4},
		{"model": "imagen-4.0-generate", "prompt": "x"},
	}
	for _, payload := range bad {
		if err := client.ValidateImage(context.Background(), payload); err == nil {
			t.Errorf("accepted %v", payload)
		}
	}
	good := []map[string]any{
		{"model": "gemini-3-pro-image", "prompt": "x", "aspect_ratio": "21:9", "input_references": refs(14)},
		{"model": "gemini-2.5-flash-image", "prompt": "x", "aspect_ratio": "auto", "input_references": refs(3)},
		{"prompt": "x", "aspect_ratio": ""},
	}
	for _, payload := range good {
		if err := client.ValidateImage(context.Background(), payload); err != nil {
			t.Errorf("rejected %v: %v", payload, err)
		}
	}
	if err := client.ValidateVideo(context.Background(), map[string]any{"model": GeminiImageModel, "prompt": "x"}); err == nil {
		t.Fatal("Gemini accepted a picture model for video")
	}
}

func TestGeminiImageRejectsRemoteReferences(t *testing.T) {
	client, calls := geminiServer(t, 200, imageAnswer(""), nil)
	_, err := client.GenerateImages(context.Background(), map[string]any{"prompt": "x", "input_references": []map[string]any{{"image_url": map[string]string{"url": "https://example.com/a.png"}}}})
	if err == nil || calls() != 0 {
		t.Fatalf("remote reference sent: err=%v calls=%d", err, calls())
	}
}
