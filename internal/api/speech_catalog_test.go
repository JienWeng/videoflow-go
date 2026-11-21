package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
