package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"videoflow-go/internal/db"
)

func TestFishSpeechWorkflowRequiresExplicitRunAndFencesDuplicates(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			calls := 0
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if fail {
					w.WriteHeader(503)
					return
				}
				w.Header().Set("Content-Type", "audio/wav")
				w.Write(voiceWAV(1))
			}))
			defer remote.Close()
			s := voiceFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("Fish speech used fal") })
			s.cfg.FishAPIKey = "fish-test"
			s.cfg.FishBaseURL = remote.URL
			s.cfg.FalAPIKey = ""
			c := voiceCharacter(t, s)
			profile := call(t, s, "PUT", "/voice/profiles/"+c, `{"revision":0,"data":{"provider":"fish","model":"s2.1-pro","mode":"saved","voice_id":"fish-voice","approved":true}}`)
			if profile.Code != 200 {
				t.Fatal(profile.Body.String())
			}
			call(t, s, "PUT", "/voice/scripts/main", fmt.Sprintf(`{"revision":0,"data":{"approved":true,"lines":[{"id":"u","character_id":%q,"language":"en","text":"Come home, Pip!"}]}}`, c))
			prepared := call(t, s, "POST", "/voice/tasks/prepare", `{"kind":"speech","script_id":"main","line_id":"u"}`)
			if prepared.Code != 200 {
				t.Fatal(prepared.Body.String())
			}
			var task db.VoiceTask
			json.Unmarshal(prepared.Body.Bytes(), &task)
			if task.Model != "fish/s2.1-pro" || calls != 0 {
				t.Fatal(task.Model, calls)
			}
			run := fmt.Sprintf(`{"confirmed":true,"fingerprint":%q}`, task.Fingerprint)
			first := call(t, s, "POST", "/voice/tasks/"+task.ID+"/run", run)
			if first.Code != 200 {
				t.Fatal(first.Code, first.Body.String())
			}
			json.Unmarshal(first.Body.Bytes(), &task)
			expected := "completed"
			if fail {
				expected = "uncertain"
			}
			if task.Status != expected {
				t.Fatal(task.Status, task.Error)
			}
			if !fail {
				var result struct {
					AssetID string `json:"asset_id"`
				}
				json.Unmarshal(task.Result, &result)
				asset, err := s.database.GetAsset(result.AssetID)
				if err != nil || !strings.Contains(string(asset.MetadataJSON), "source_bindings") {
					t.Fatal("speech provenance missing", err)
				}
			}
			again := call(t, s, "POST", "/voice/tasks/"+task.ID+"/run", run)
			if again.Code != 409 || calls != 1 {
				t.Fatal(again.Code, calls)
			}
			refreshed := call(t, s, "POST", "/voice/tasks/"+task.ID+"/refresh", `{}`)
			want := 200
			if fail {
				want = 409
			}
			if refreshed.Code != want || calls != 1 {
				t.Fatal("refresh resubmitted or lost state", refreshed.Code, calls)
			}
		})
	}
}

func TestFishSettingsKeepKeyPrivateAndRejectForeignEndpoint(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("configuration check contacted provider") })
	saved := call(t, s, "PUT", "/settings/providers/fish", `{"api_key":"fish-private-test-secret","base_url":"https://api.fish.audio"}`)
	if saved.Code != 200 || strings.Contains(saved.Body.String(), "fish-private-test-secret") {
		t.Fatal(saved.Code, saved.Body.String())
	}
	checked := call(t, s, "POST", "/settings/providers/fish/test", `{}`)
	if checked.Code != 200 || !strings.Contains(checked.Body.String(), `"generation_tested":false`) {
		t.Fatal(checked.Code, checked.Body.String())
	}
	bad := call(t, s, "PUT", "/settings/providers/fish", `{"api_key":"private","base_url":"https://foreign.invalid"}`)
	if bad.Code != 400 {
		t.Fatal(bad.Code)
	}
	c := voiceCharacter(t, s)
	bad = call(t, s, "PUT", "/voice/profiles/"+c, `{"revision":0,"data":{"provider":"fish","model":"invented","voice_id":"id","approved":true}}`)
	if bad.Code != 400 {
		t.Fatal("unknown model saved", bad.Code)
	}
}
