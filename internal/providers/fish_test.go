package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFishSpeechUsesExplicitVoiceModelAndBearer(t *testing.T) {
	calls := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/tts" || r.Header.Get("model") != "s2.1-pro" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("wrong Fish contract")
		}
		var p map[string]any
		json.NewDecoder(r.Body).Decode(&p)
		if p["reference_id"] != "voice-123" || p["text"] != "Hello" || p["format"] != "mp3" {
			t.Errorf("wrong payload %v", p)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write([]byte("audio"))
	}))
	defer remote.Close()
	c := NewFishClientAt("test-key", remote.URL)
	if _, err := c.Speech(context.Background(), "s2.1-pro", map[string]any{"text": "Hello", "reference_id": "voice-123", "format": "mp3"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Speech(context.Background(), "unknown", map[string]any{"text": "Hello", "reference_id": "voice-123"}); err == nil {
		t.Fatal("unknown model accepted")
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}
func TestFishSpeechErrorsNeverRetryOrExposeCredentials(t *testing.T) {
	calls := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(402)
		w.Write([]byte("private-key"))
	}))
	defer remote.Close()
	_, err := NewFishClientAt("private-key", remote.URL).Speech(context.Background(), "s2.1-pro", map[string]any{"text": "Hi", "reference_id": "saved"})
	if err == nil || calls != 1 {
		t.Fatal(err, calls)
	}
	if err.Error() != "Fish Audio returned HTTP 402; check account access and voice model ID" {
		t.Fatal(err)
	}
}
