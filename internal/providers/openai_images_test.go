package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIImageGenerationAndReferenceEdit(t *testing.T) {
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j4v8AAAAASUVORK5CYII="
	for _, withReference := range []bool{false, true} {
		requests := 0
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer fixture" {
				t.Error("incorrect OpenAI request method or authentication")
			}
			if withReference {
				if r.URL.Path != "/v1/images/edits" {
					t.Errorf("unexpected edit path: %s", r.URL.Path)
				}
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				defer r.MultipartForm.RemoveAll()
				if r.FormValue("model") != OpenAIImageModel || r.FormValue("size") != "864x1536" || len(r.MultipartForm.File["image[]"]) != 1 {
					t.Error("missing model, portrait size or attached reference")
				}
			} else {
				var fields map[string]string
				if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
					t.Error(err)
				}
				if r.URL.Path != "/v1/images/generations" || fields["size"] != "864x1536" || fields["model"] != OpenAIImageModel {
					t.Error("incorrect generation request")
				}
			}
			_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + png + `"}]}`))
		}))
		client := NewOpenAIImages("fixture", server.URL+"/v1", OpenAIImageModel)
		client.http.client = server.Client()
		payload := map[string]any{"aspect_ratio": "9:16", "prompt": "A small alien explores Kuala Lumpur"}
		if withReference {
			payload["input_references"] = []map[string]any{{"image_url": map[string]string{"url": "data:image/png;base64," + png}}}
		}
		images, err := client.GenerateImages(context.Background(), payload)
		server.Close()
		if err != nil || len(images) != 1 || images[0].MediaType != "image/png" || requests != 1 {
			t.Fatalf("OpenAI fallback failed: images=%d, requests=%d, error=%v", len(images), requests, err)
		}
	}
}
