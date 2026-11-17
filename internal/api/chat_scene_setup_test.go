package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"videoflow-go/internal/models"
)

func TestSceneChatSetupSavesAllMissingVoicesAtomically(t *testing.T) {
	for _, variant := range []string{"success", "partial", "stale_profile", "stale_scene", "ready_overwrite", "foreign"} {
		t.Run(variant, func(t *testing.T) {
			calls := 0
			s := voiceFixture(t, func(w http.ResponseWriter, r *http.Request) { calls++; t.Error("setup made a paid provider request") })
			s.cfg.UseLocalText = true
			s.cfg.FishAPIKey = "test"
			project, _ := s.database.GetActiveProjectID()
			ids := []string{}
			for _, name := range []string{"Pip", "Mama", "Shopkeeper"} {
				c := &models.Character{ProjectID: &project, Name: name}
				if err := s.database.CreateCharacter(c); err != nil {
					t.Fatal(err)
				}
				ids = append(ids, c.ID)
			}
			s.database.SaveVoiceDocument(project, "profile", ids[0], 0, voiceJSON(VoiceProfile{CharacterID: ids[0], Provider: "fish", Model: "s2.1-pro", Mode: "saved", VoiceID: "keep-existing", Approved: true}))
			lines := []SpokenLine{}
			for _, id := range ids {
				lines = append(lines, SpokenLine{ID: id, CharacterID: id, Text: "Hello", Language: "en"})
			}
			scene := &models.Scene{ProjectID: &project, Title: "Pawprints to Mama", Duration: 8, AspectRatio: "9:16", CharacterIDsJSON: voiceJSON(ids), SceneJSON: voiceJSON(map[string]any{"spoken_lines": lines})}
			if err := s.database.CreateScene(scene); err != nil {
				t.Fatal(err)
			}
			scene, _ = s.database.GetScene(scene.ID)
			state := call(t, s, "GET", "/voice/scenes/"+scene.ID+"/setup", "")
			if state.Code != 200 {
				t.Fatal(state.Body.String())
			}
			var setup struct {
				Speakers []sceneSpeakerSetup `json:"speakers"`
				Ready    bool                `json:"voices_ready"`
			}
			json.Unmarshal(state.Body.Bytes(), &setup)
			if len(setup.Speakers) != 3 || setup.Ready {
				t.Fatal(state.Body.String())
			}
			updates := []map[string]any{}
			for _, id := range ids[1:] {
				updates = append(updates, map[string]any{"character_id": id, "revision": 0, "provider": "fish", "model": "s2.1-pro", "voice_id": "voice-" + id})
			}
			expected := 200
			switch variant {
			case "partial":
				updates = updates[:1]
				expected = 400
			case "stale_profile":
				s.database.SaveVoiceDocument(project, "profile", ids[2], 0, voiceJSON(VoiceProfile{Provider: "fish", Mode: "saved", VoiceID: "unapproved", Approved: false}))
				expected = 409
			case "stale_scene":
				scene.Summary = "Concurrent edit"
				s.database.UpdateScene(scene)
				expected = 409
			case "ready_overwrite":
				updates = append(updates, map[string]any{"character_id": ids[0], "revision": 1, "provider": "fish", "model": "s2.1-pro", "voice_id": "replace"})
				expected = 400
			case "foreign":
				other, _ := s.database.CreateProject("Other", "")
				c := &models.Character{ProjectID: &other.ID, Name: "Foreign"}
				s.database.CreateCharacter(c)
				updates[0]["character_id"] = c.ID
				expected = 400
			}
			// The original GET's displayed timestamp is the save precondition.
			var snapshot map[string]any
			json.Unmarshal(state.Body.Bytes(), &snapshot)
			body, _ := json.Marshal(map[string]any{"confirmed": true, "expected_updated_at": snapshot["updated_at"], "profiles": updates})
			result := call(t, s, "PUT", "/voice/scenes/"+scene.ID+"/setup", string(body))
			if result.Code != expected {
				t.Fatal(result.Code, result.Body.String())
			}
			first, _ := s.database.GetVoiceDocument(project, "profile", ids[0])
			if first.Revision != 1 || !strings.Contains(string(first.Data), "keep-existing") {
				t.Fatal("ready voice changed", string(first.Data))
			}
			if variant == "success" {
				if !strings.Contains(result.Body.String(), `"voices_ready":true`) {
					t.Fatal(result.Body.String())
				}
				for _, id := range ids[1:] {
					doc, err := s.database.GetVoiceDocument(project, "profile", id)
					if err != nil || doc.Revision != 1 {
						t.Fatal(doc, err)
					}
				}
			} else {
				if _, err := s.database.GetVoiceDocument(project, "profile", ids[1]); err == nil {
					t.Fatal("failed batch partially saved")
				}
			}
			if calls != 0 {
				t.Fatal(calls)
			}
		})
	}
}
func TestSceneChatSetupUsesOnlyActualSpeakers(t *testing.T) {
	s := voiceFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected provider call") })
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	first := voiceCharacter(t, s)
	unused := voiceCharacter(t, s)
	scene := &models.Scene{ProjectID: &project, Title: "Actual speakers", CharacterIDsJSON: voiceJSON([]string{first, unused}), SceneJSON: voiceJSON(map[string]any{"spoken_lines": []SpokenLine{{ID: "one", CharacterID: first, Text: "Hello", Language: "English"}, {ID: "two", CharacterID: first, Text: "Again", Language: "English"}}})}
	s.database.CreateScene(scene)
	response := call(t, s, "GET", "/voice/scenes/"+scene.ID+"/setup", "")
	var state struct {
		Speakers []sceneSpeakerSetup `json:"speakers"`
		Language string              `json:"language"`
	}
	json.Unmarshal(response.Body.Bytes(), &state)
	if response.Code != 200 || len(state.Speakers) != 1 || state.Speakers[0].ID != first || state.Language != "en" {
		t.Fatal(response.Body.String())
	}
	other, _ := s.database.CreateProject("Elsewhere", "")
	foreign := &models.Scene{ProjectID: &other.ID, Title: "Not owned"}
	s.database.CreateScene(foreign)
	if r := call(t, s, "GET", "/voice/scenes/"+foreign.ID+"/setup", ""); r.Code != 404 {
		t.Fatal(fmt.Sprint(r.Code))
	}
}
