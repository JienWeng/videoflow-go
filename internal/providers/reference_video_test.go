package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFalReferenceVideoForwardsAllModalities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+FalNativeVoiceModel {
			t.Errorf("wrong model route %s", r.URL.Path)
		}
		var p map[string]any
		json.NewDecoder(r.Body).Decode(&p)
		for _, key := range []string{"image_urls", "video_urls", "audio_urls"} {
			if len(p[key].([]any)) != 1 {
				t.Error("dropped reference", key)
			}
		}
		if p["image_url"] != nil || p["end_image_url"] != nil {
			t.Error("mixed strict-frame mode")
		}
		w.Write([]byte(`{"request_id":"reference-job"}`))
	}))
	defer server.Close()
	client := NewFalClientAt("fixture", server.URL, FalNativeVoiceModel)
	p := map[string]any{"model": FalNativeVoiceModel, "prompt": "Use reference motion", "duration": 8, "aspect_ratio": "9:16", "resolution": "720p", "image_urls": []string{"data:image/png;base64,aA=="}, "video_urls": []string{"data:video/mp4;base64,aA=="}, "audio_urls": []string{"data:audio/wav;base64,aA=="}}
	if _, e := client.CreateVideo(context.Background(), p); e != nil {
		t.Fatal(e)
	}
	p["frame_images"] = []map[string]any{{"frame_type": "first_frame", "image_url": map[string]string{"url": "data:image/png;base64,aA=="}}}
	if e := client.ValidateVideo(context.Background(), p); e == nil {
		t.Fatal("strict-frame and reference modes were mixed")
	}
}
