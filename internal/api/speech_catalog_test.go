package api

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"videoflow-go/internal/providers"
)

func TestSpeechCatalogRefreshMatchAndPinnedElevenGeneration(t *testing.T) {
	modelCalls, speechCalls := 0, 0
	latest := "eleven_new_v4"
	outage := false
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("xi-api-key") != "test-eleven" {
			t.Error("wrong auth header")
		}
		switch r.URL.Path {
		case "/v1/models":
			modelCalls++
			if outage {
				w.WriteHeader(503)
				return
			}
			json.NewEncoder(w).Encode([]map[string]any{{"model_id": "eleven_multilingual_v2", "name": "Stable", "can_do_text_to_speech": true, "languages": []map[string]string{{"language_id": "en"}}}, {"model_id": latest, "name": "New catalog model", "can_do_text_to_speech": true}, {"model_id": "not_speech", "can_do_text_to_speech": false}})
		case "/v2/voices":
			if r.URL.Query().Get("language") != "en" {
				t.Error("language filter missing")
			}
			w.Write([]byte(`{"voices":[{"voice_id":"z","name":"Brisk","description":"Energetic delivery"},{"voice_id":"warm_voice","name":"Warm","description":"gentle warm caring narrator","labels":{"language":"en"}}],"has_more":true,"next_page_token":"next-token"}`))
		case "/v1/text-to-speech/warm_voice":
			speechCalls++
			var in map[string]any
			json.NewDecoder(r.Body).Decode(&in)
			if in["model_id"] != "eleven_multilingual_v2" {
				t.Errorf("prepared model changed: %v", in)
			}
			w.WriteHeader(500)
		default:
			t.Error("unexpected endpoint", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer remote.Close()
	s := voiceFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("ElevenLabs used fal") })
	s.cfg.ElevenAPIKey = "test-eleven"
	s.cfg.ElevenBaseURL = remote.URL
	c := voiceCharacter(t, s)
	ch, _ := s.database.GetCharacter(c)
	ch.Description = "A gentle warm caring shopkeeper"
	s.database.UpdateCharacter(ch)
	for i := 0; i < 2; i++ {
		r := call(t, s, "GET", "/voice/catalog/elevenlabs/models", "")
		if r.Code != 200 || strings.Contains(r.Body.String(), "not_speech") {
			t.Fatal(r.Code, r.Body.String())
		}
	}
	if modelCalls != 1 {
		t.Fatal("catalog not cached", modelCalls)
	}
	latest = "eleven_future_v5"
	r := call(t, s, "GET", "/voice/catalog/elevenlabs/models?refresh=true", "")
	if r.Code != 200 || !strings.Contains(r.Body.String(), latest) || modelCalls != 2 {
		t.Fatal(r.Body.String(), modelCalls)
	}
	r = call(t, s, "GET", "/voice/catalog/elevenlabs/voices?character_id="+c+"&language=en", "")
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"id":"warm_voice"`) || !strings.Contains(r.Body.String(), "next-token") {
		t.Fatal(r.Body.String())
	}
	var match struct {
		Suggested providers.SpeechVoice
		Reason    string
	}
	json.Unmarshal(r.Body.Bytes(), &match)
	if match.Suggested.ID != "warm_voice" || !strings.Contains(match.Reason, "gentle") {
		t.Fatal(match)
	}
	outage = true
	stale := call(t, s, "GET", "/voice/catalog/elevenlabs/models?refresh=true", "")
	if stale.Code != 200 || !strings.Contains(stale.Body.String(), `"stale":true`) {
		t.Fatal("outage hid stale catalog", stale.Body.String())
	}
	outage = false
	pref := call(t, s, "PUT", "/voice/preferences", `{"primary_provider":"elevenlabs"}`)
	if pref.Code != 200 {
		t.Fatal(pref.Body.String())
	}
	r = call(t, s, "PUT", "/voice/profiles/"+c, `{"revision":0,"data":{"provider":"elevenlabs","model":"auto","voice_id":"warm_voice","approved":true}}`)
	if r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	state := call(t, s, "GET", "/voice/state", "")
	if state.Code != 200 || !strings.Contains(state.Body.String(), `"primary_provider":"elevenlabs"`) || !strings.Contains(state.Body.String(), "warm_voice") || strings.Contains(state.Body.String(), "tasks") {
		t.Fatal(state.Body.String())
	}
	if r = call(t, s, "PUT", "/voice/preferences", `{"primary_provider":"fal"}`); r.Code != 400 {
		t.Fatal("removed fal voice provider accepted", r.Code)
	}
	if r = call(t, s, "PUT", "/voice/profiles/"+c, `{"revision":1,"data":{"provider":"fal","voice_id":"warm_voice","approved":true}}`); r.Code != 400 {
		t.Fatal("removed fal voice provider saved on a profile", r.Code)
	}
	if speechCalls != 0 {
		t.Fatal("saving a voice made speech", speechCalls)
	}
	if _, err := s.resolveSpeechModel(context.Background(), "elevenlabs", "invented"); err == nil {
		t.Fatal("invented model accepted")
	}
}
func TestVoiceMatchDeterministicAndHonestWithoutMatches(t *testing.T) {
	v, reason := voiceMatch([]providers.SpeechVoice{{ID: "b", Name: "One"}, {ID: "a", Name: "Two"}}, "unmatched trait")
	if v.ID != "a" || !strings.Contains(reason, "No descriptive match") {
		t.Fatal(v, reason)
	}
}

// Google Gemini speaks with its prebuilt voices on the saved Gemini key; picking one
// asks Google nothing.
func TestGeminiVoicesUseTheGeminiKey(t *testing.T) {
	s := voiceFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("Gemini voices used fal") })
	s.cfg.GeminiAPIKey = ""
	c := voiceCharacter(t, s)
	if r := call(t, s, "PUT", "/voice/preferences", `{"primary_provider":"gemini"}`); r.Code != 400 {
		t.Fatalf("Gemini chosen without a key: %d", r.Code)
	}
	setup := call(t, s, "GET", "/ai/setup", "")
	if !strings.Contains(setup.Body.String(), `{"id":"gemini","label":"Google Gemini","ready":false}`) {
		t.Fatalf("voice options = %s", setup.Body.String())
	}
	s.cfg.GeminiAPIKey = "gemini-fixture"
	models := call(t, s, "GET", "/voice/catalog/gemini/models", "")
	if models.Code != 200 || !strings.Contains(models.Body.String(), `"id":"gemini-3.8-flash-tts"`) || strings.Contains(models.Body.String(), "2.5") {
		t.Fatalf("models = %d %s", models.Code, models.Body.String())
	}
	voices := call(t, s, "GET", "/voice/catalog/gemini/voices?character_id="+c+"&search=firm", "")
	if voices.Code != 200 || !strings.Contains(voices.Body.String(), `"id":"Kore"`) || strings.Contains(voices.Body.String(), `"id":"Puck"`) {
		t.Fatalf("voices = %d %s", voices.Code, voices.Body.String())
	}
	if r := call(t, s, "PUT", "/voice/preferences", `{"primary_provider":"gemini"}`); r.Code != 200 {
		t.Fatalf("choose gemini: %d %s", r.Code, r.Body.String())
	}
	if r := call(t, s, "PUT", "/voice/profiles/"+c, `{"revision":0,"data":{"provider":"gemini","model":"auto","voice_id":"Kore","approved":true}}`); r.Code != 200 {
		t.Fatalf("save voice: %d %s", r.Code, r.Body.String())
	}
	if model, err := s.resolveSpeechModel(context.Background(), "gemini", "auto", "ms"); err != nil || model != "gemini-3.8-flash-tts" {
		t.Fatalf("auto model = %q %v", model, err)
	}
	state := call(t, s, "GET", "/voice/state", "")
	if !strings.Contains(state.Body.String(), `"primary_provider":"gemini"`) {
		t.Fatalf("state = %s", state.Body.String())
	}
	if !strings.Contains(call(t, s, "GET", "/ai/setup", "").Body.String(), `"provider":"gemini","ready":true,"service":"Google Gemini"`) {
		t.Fatal("AI setup does not show Gemini as the voice")
	}
}

// Google's WAV speech is kept as MP3 like every other voice line.
func TestGeminiSpeechWAVBecomesMP3(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	samples := 24000 / 2 // half a second of 24 kHz mono 16-bit silence
	wav := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(36+samples*2))...)
	wav = append(wav, "WAVEfmt "...)
	wav = binary.LittleEndian.AppendUint32(wav, 16)
	wav = binary.LittleEndian.AppendUint16(wav, 1)
	wav = binary.LittleEndian.AppendUint16(wav, 1)
	wav = binary.LittleEndian.AppendUint32(wav, 24000)
	wav = binary.LittleEndian.AppendUint32(wav, 48000)
	wav = binary.LittleEndian.AppendUint16(wav, 2)
	wav = binary.LittleEndian.AppendUint16(wav, 16)
	wav = append(wav, "data"...)
	wav = binary.LittleEndian.AppendUint32(wav, uint32(samples*2))
	wav = append(wav, make([]byte, samples*2)...)
	mp3, err := wavToMP3(context.Background(), wav)
	if err != nil || len(mp3) == 0 || http.DetectContentType(mp3) != "audio/mpeg" {
		t.Fatalf("mp3 = %d bytes (%s), %v", len(mp3), http.DetectContentType(mp3), err)
	}
	if _, err = wavToMP3(context.Background(), []byte("not audio")); err == nil {
		t.Fatal("garbage became MP3")
	}
}
