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

func TestGeminiTranscribeReadsWordTimes(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/v1beta/interactions" || r.URL.RawQuery != "" || r.Header.Get("x-goog-api-key") != "fixture-key" {
			t.Errorf("request %s %s", r.Method, r.URL)
		}
		var body map[string]any
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		config, _ := json.Marshal(body["generation_config"])
		if body["model"] != GeminiTranscribeModel || string(config) != `{"transcription_config":{"language_codes":["ms"],"mode":{"timestamp_granularities":["word"],"type":"verbatim"}}}` {
			t.Errorf("model %v config %s", body["model"], config)
		}
		input := body["input"].([]any)[0].(map[string]any)
		if input["type"] != "audio" || input["mime_type"] != "audio/wav" || input["data"] == "" {
			t.Errorf("input = %v", input)
		}
		_, _ = w.Write([]byte(`{"id":"interactions/abc123xyz","status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"Hello world",
			"annotations":[{"type":"word_info","text":"Hello","speaker":"spk_1","start_offset":"0.100s","end_offset":"0.400s"},
			{"type":"word_info","text":"world","speaker":"spk_1","start_offset":"0.500s","end_offset":"0.800s"},
			{"type":"word_info","text":"bad","start_offset":"0.9","end_offset":"1.0s"},{"type":"other","text":"x","start_offset":"1s","end_offset":"2s"}]}]}]}`))
	}))
	defer server.Close()
	words, err := GeminiTranscribeWords(context.Background(), "fixture-key", server.URL+"/v1beta", geminiWAV, "ms")
	if err != nil || len(words) != 2 || words[0] != (SpokenWord{"Hello", 0.1, 0.4}) || words[1] != (SpokenWord{"world", 0.5, 0.8}) {
		t.Fatalf("words = %v %v", words, err)
	}
	if _, err = GeminiTranscribeWords(context.Background(), "k", server.URL+"/v1beta", []byte("ID3 not a wav file at all"), ""); err == nil || calls != 1 {
		t.Fatalf("non-WAV sent: %v calls=%d", err, calls)
	}
}

func TestGeminiTranscribeReportsUnfinishedAnswers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"x","status":"failed","errors":[{"code":"x","message":"y"}]}`))
	}))
	defer server.Close()
	if _, err := GeminiTranscribeWords(context.Background(), "k", server.URL+"/v1beta", geminiWAV, ""); err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("err = %v", err)
	}
}
