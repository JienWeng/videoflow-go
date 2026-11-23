package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A 44-byte WAV header: what Go's sniffer reads as audio/wave.
var geminiWAV = append([]byte("RIFF\x24\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\xc0\x5d\x00\x00\x80\xbb\x00\x00\x02\x00\x10\x00data\x00\x00\x00\x00"), 0, 0)

func speechAnswer(content string) string {
	return `{"id":"v1_tts","status":"completed","steps":[{"type":"user_input"},{"type":"model_output","content":[` + content + `]}]}`
}

func geminiSpeechServer(t *testing.T, status int, body string, inspect func(*http.Request, map[string]any)) (string, *int) {
	t.Helper()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request map[string]any
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &request)
		if inspect != nil {
			inspect(r, request)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server.URL + "/v1beta", &calls
}

func TestGeminiSpeechSendsOneVoiceAndReturnsWAV(t *testing.T) {
	audio := base64.StdEncoding.EncodeToString(geminiWAV)
	base, calls := geminiSpeechServer(t, 200, speechAnswer(`{"type":"audio","mime_type":"audio/wav","data":"`+audio+`"}`), func(r *http.Request, body map[string]any) {
		if r.Method != "POST" || r.URL.Path != "/v1beta/interactions" || r.URL.RawQuery != "" || r.Header.Get("x-goog-api-key") != "fixture-key" || r.Header.Get("Authorization") != "" {
			t.Errorf("request %s %s key=%q", r.Method, r.URL, r.Header.Get("x-goog-api-key"))
		}
		got, _ := json.Marshal(body)
		want := `{"generation_config":{"speech_config":[{"voice":"Kore"}]},"input":[{"content":[{"text":"Hello there.","type":"text"}],"type":"user_input"}],"model":"gemini-3.8-flash-tts","response_format":{"type":"audio"}}`
		if string(got) != want {
			t.Errorf("body = %s", got)
		}
	})
	wav, err := GeminiSpeech(context.Background(), "fixture-key", base, "gemini-3.8-flash-tts", "Kore", " Hello there. ")
	if err != nil || string(wav) != string(geminiWAV) || *calls != 1 {
		t.Fatalf("wav=%d err=%v calls=%d", len(wav), err, *calls)
	}
}

func TestGeminiSpeechRefusesBadRequestsAndAnswers(t *testing.T) {
	base, calls := geminiSpeechServer(t, 200, speechAnswer(`{"type":"text","text":"no"}`), nil)
	for _, c := range [][3]string{
		{"gemini-2.5-flash-preview-tts", "Kore", "hi"},
		{"gemini-3.8-flash-tts", "Rachel", "hi"},
		{"gemini-3.8-flash-tts", "Kore", " "},
		{"gemini-3.8-flash-tts", "Kore", strings.Repeat("a", 5001)},
	} {
		if _, err := GeminiSpeech(context.Background(), "k", base, c[0], c[1], c[2]); err == nil {
			t.Errorf("accepted %v", c[0:2])
		}
	}
	if *calls != 0 {
		t.Fatalf("%d bad requests reached Google", *calls)
	}
	if _, err := GeminiSpeech(context.Background(), "k", base, "gemini-3.8-flash-tts", "Puck", "hi"); err == nil || !strings.Contains(err.Error(), "no") {
		t.Fatalf("answer without audio = %v", err)
	}
	mp3 := base64.StdEncoding.EncodeToString([]byte("ID3\x03\x00\x00\x00\x00\x00\x00 not wav at all, long enough to sniff........"))
	base, _ = geminiSpeechServer(t, 200, speechAnswer(`{"type":"audio","data":"`+mp3+`"}`), nil)
	if _, err := GeminiSpeech(context.Background(), "k", base, "gemini-3.8-flash-tts", "Puck", "hi"); err == nil {
		t.Fatal("non-WAV audio accepted")
	}
	base, _ = geminiSpeechServer(t, 403, `{"error":{"code":403,"message":"Method doesn't allow unregistered callers","status":"PERMISSION_DENIED"}}`, nil)
	if _, err := GeminiSpeech(context.Background(), "k", base, "gemini-3.8-flash-tts", "Puck", "hi"); err == nil || !strings.Contains(err.Error(), "PERMISSION_DENIED Method") {
		t.Fatalf("error = %v", err)
	}
}

func TestGeminiSpeechCatalog(t *testing.T) {
	models, err := SpeechModels(context.Background(), "gemini", "", "", "")
	if err != nil || len(models) != 2 || !models[0].Default || models[0].ID != "gemini-3.8-flash-tts" {
		t.Fatalf("models = %v %v", models, err)
	}
	all, _ := ListSpeechVoices(context.Background(), "gemini", "", "", "", "", "")
	if len(all.Voices) != 30 || all.Next != "" {
		t.Fatalf("voices = %d", len(all.Voices))
	}
	warm, _ := ListSpeechVoices(context.Background(), "gemini", "", "", "warm", "", "")
	if len(warm.Voices) != 1 || warm.Voices[0].ID != "Sulafat" || warm.Voices[0].Description != "Warm" {
		t.Fatalf("warm = %v", warm.Voices)
	}
	for _, v := range all.Voices {
		if !SafeSpeechID(v.ID) {
			t.Errorf("voice %q is not a safe ID", v.ID)
		}
	}
}
