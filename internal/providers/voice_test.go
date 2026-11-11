package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVoiceQueuePreservesExplicitVoiceAndUsesQueueRoot(t *testing.T) {
	var submitted map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Key secret" {
			t.Error("missing auth")
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /fal-ai/minimax/speech-02-hd":
			json.NewDecoder(r.Body).Decode(&submitted)
			w.Write([]byte(`{"request_id":"voice-123"}`))
		case "GET /fal-ai/minimax/requests/voice-123/status":
			w.Write([]byte(`{"status":"COMPLETED"}`))
		case "GET /fal-ai/minimax/requests/voice-123":
			w.Write([]byte(`{"audio":{"url":"https://fal.media/test.mp3"}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := NewFalClientAt("secret", srv.URL, "")
	id, err := c.SubmitVoice(context.Background(), FalSpeechModel, map[string]any{"text": "Exact words.", "voice_setting": map[string]any{"voice_id": "my_voice"}})
	if err != nil || id != "voice-123" {
		t.Fatalf("submit %s %v", id, err)
	}
	state, data, err := c.VoiceResult(context.Background(), FalSpeechModel, id)
	if err != nil || state != "completed" || len(data) == 0 {
		t.Fatalf("result %s %s %v", state, data, err)
	}
	if submitted["voice_setting"].(map[string]any)["voice_id"] != "my_voice" {
		t.Fatal(submitted)
	}
}
func TestVoiceRejectsUncuratedEndpointAndMissingVoice(t *testing.T) {
	c := NewFalClientAt("", "http://127.0.0.1:1", "")
	for _, tc := range []struct {
		model string
		p     map[string]any
	}{{"untrusted/endpoint", map[string]any{}}, {FalSpeechModel, map[string]any{"text": "hi"}}, {FalAlignmentModel, map[string]any{"text": "hi"}}} {
		if _, err := c.SubmitVoice(context.Background(), tc.model, tc.p); err == nil {
			t.Fatal("accepted invalid voice request")
		}
	}
}
