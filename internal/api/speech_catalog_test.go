package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"videoflow-go/internal/db"
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
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Write(voiceWAV(1))
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
	call(t, s, "PUT", "/voice/scripts/main", fmt.Sprintf(`{"revision":0,"data":{"approved":true,"lines":[{"id":"u","character_id":%q,"language":"en","text":"Come home, Pip!"}]}}`, c))
	r = call(t, s, "POST", "/voice/tasks/prepare", `{"kind":"speech","script_id":"main","line_id":"u"}`)
	if r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	var task db.VoiceTask
	json.Unmarshal(r.Body.Bytes(), &task)
	if task.Model != "elevenlabs/eleven_multilingual_v2" || speechCalls != 0 {
		t.Fatal(task.Model, speechCalls)
	}
	result := call(t, s, "POST", "/voice/tasks/"+task.ID+"/run", fmt.Sprintf(`{"confirmed":true,"fingerprint":%q}`, task.Fingerprint))
	if result.Code != 200 || !strings.Contains(result.Body.String(), `"status":"completed"`) || speechCalls != 1 {
		t.Fatal(result.Body.String(), speechCalls)
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
