package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"videoflow-go/internal/models"
)

func TestChatReturnsRunnableCanonicalActions(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"add_scene", "generate_script"}, {"create_script", "generate_script"}, {"generate_shots", "generate_shots"}, {"none", "unknown"}, {"invented_action", "unknown"}} {
		t.Run(tc.input, func(t *testing.T) {
			s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/models" {
					w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
					return
				}
				content, _ := json.Marshal(map[string]string{"reply": "Scene idea", "action": tc.input})
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(content)}}}})
			})
			result := call(t, s, "POST", "/chat", `{"message":"Write a new scene"}`)
			if result.Code != 200 {
				t.Fatalf("%d %s", result.Code, result.Body.String())
			}
			var body struct{ Intent struct{ Action string } }
			json.Unmarshal(result.Body.Bytes(), &body)
			if body.Intent.Action != tc.want {
				t.Fatalf("dead card action %q; want %q", body.Intent.Action, tc.want)
			}
		})
	}
}

func TestChatRejectsMissingSelectedNodeBeforeInference(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("must not invoke model for invalid selection") })
	result := call(t, s, "POST", "/chat", `{"message":"Change this","selected":{"kind":"shot","id":"missing"}}`)
	if result.Code != 409 {
		t.Fatalf("got %d %s", result.Code, result.Body.String())
	}
}

func TestChatUsesOwnedSelectionAndPinsCompatibleAction(t *testing.T) {
	var sawContext bool
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
			return
		}
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		raw, _ := json.Marshal(payload)
		sawContext = strings.Contains(string(raw), "name: studio-recovery") && strings.Contains(string(raw), "recent_activity") && strings.Contains(string(raw), "selected_item") && strings.Contains(string(raw), "crimson car") && strings.Contains(string(raw), "canvas") && strings.Contains(string(raw), "workflow_policy") && strings.Contains(string(raw), "render_setup") && strings.Contains(string(raw), "Never tell the user to approve images") && strings.Contains(string(raw), "connected camera test")
		content := `{"reply":"Refine selected scene","action":"refine_scene","scene_id":"wrong-scene"}`
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	})
	created := call(t, s, "POST", "/scenes", `{"title":"Car scene","summary":"crimson car"}`)
	var scene struct {
		ID string `json:"id"`
	}
	json.Unmarshal(created.Body.Bytes(), &scene)
	shot := call(t, s, "POST", "/scenes/"+scene.ID+"/shots", `{"shot_order":1,"duration":4,"prompt":"connected camera test"}`)
	if shot.Code != 201 {
		t.Fatalf("seed shot: %d %s", shot.Code, shot.Body.String())
	}
	payload, _ := json.Marshal(map[string]any{"message": "Make the lighting warmer", "selected": map[string]string{"kind": "scene", "id": scene.ID}})
	result := call(t, s, "POST", "/chat", string(payload))
	if result.Code != 200 {
		t.Fatalf("%d %s", result.Code, result.Body.String())
	}
	var body struct {
		Intent struct {
			SceneID string `json:"scene_id"`
		}
	}
	json.Unmarshal(result.Body.Bytes(), &body)
	if !sawContext || body.Intent.SceneID != scene.ID {
		t.Fatalf("selection not honored: context=%v response=%s", sawContext, result.Body.String())
	}
}

func TestScopedChatRefusesWrongKindAction(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
			return
		}
		content := `{"reply":"Render another scene","action":"render_scene"}`
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	})
	created := call(t, s, "POST", "/characters", `{"name":"Alice"}`)
	var c struct {
		ID string `json:"id"`
	}
	json.Unmarshal(created.Body.Bytes(), &c)
	payload, _ := json.Marshal(map[string]any{"message": "Make this red", "selected": map[string]string{"kind": "character", "id": c.ID}})
	result := call(t, s, "POST", "/chat", string(payload))
	if result.Code != 200 {
		t.Fatal(result.Body.String())
	}
	var body struct{ Intent struct{ Action string } }
	json.Unmarshal(result.Body.Bytes(), &body)
	if body.Intent.Action != "unknown" {
		t.Fatalf("wrong-scope runnable action: %s", result.Body.String())
	}
}

func TestOpenEditorActionIsFreeNavigationWithKnownPanel(t *testing.T) {
	if canonicalChatAction("open_editor") != "open_editor" || !selectedChatActionAllowed("output", "open_editor") || !selectedChatActionAllowed("scene", "open_editor") {
		t.Fatal("open_editor must be a supported action for any selection")
	}
}

func fakeChatModel(content string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	}
}

func TestChatUpdateCharacterKeepsOnlyRealFieldsForTheResolvedCharacter(t *testing.T) {
	s := routerServer(t, fakeChatModel(`{"reply":"Here is the new look.","action":"update_character","character_changes":{"appearance":"  Enormous fluffy all-black cat, green eyes  ","personality":"   "}}`))
	call(t, s, "POST", "/characters", `{"name":"Giant Cat","appearance":"Orange tabby"}`)
	result := call(t, s, "POST", "/chat", `{"message":"make the cat black"}`)
	var body struct {
		Intent struct {
			Action           string            `json:"action"`
			CharacterID      string            `json:"character_id"`
			CharacterChanges map[string]string `json:"character_changes"`
		}
	}
	if result.Code != 200 || json.Unmarshal(result.Body.Bytes(), &body) != nil {
		t.Fatal(result.Code, result.Body.String())
	}
	if body.Intent.Action != "update_character" || body.Intent.CharacterID == "" || body.Intent.CharacterChanges["appearance"] != "Enormous fluffy all-black cat, green eyes" || len(body.Intent.CharacterChanges) != 1 {
		t.Fatalf("%+v", body.Intent)
	}
}

func TestChatStoryQuestionFollowsTheUsersLanguage(t *testing.T) {
	for _, tc := range []struct{ lang, reply, want string }{
		{"es", "¿Qué estilo visual prefieres?", "¿Qué estilo visual prefieres?"},
		{"en", "Which style?", "Before I create the story"},
		{"", "Which style?", "Before I create the story"},
	} {
		content, _ := json.Marshal(map[string]any{"reply": tc.reply, "action": "generate_script", "user_language": tc.lang, "creation_brief": map[string]any{"idea": "Un gato que baila"}})
		s := routerServer(t, fakeChatModel(string(content)))
		result := call(t, s, "POST", "/chat", `{"message":"hazme un video de un gato que baila"}`)
		var body struct{ Intent struct{ Reply string } }
		json.Unmarshal(result.Body.Bytes(), &body)
		if !strings.HasPrefix(body.Intent.Reply, tc.want) {
			t.Fatalf("lang %q: reply %q", tc.lang, body.Intent.Reply)
		}
	}
}

func TestChatMakeClipUsesAttachedPhotoAndRefusesWithoutOne(t *testing.T) {
	answer := `{"reply":"I'll make him dance.","action":"make_clip","clip_prompt":"The man from the image dances energetically","clip_duration":99,"clip_aspect":"4:5","clip_audio":true}`
	s := routerServer(t, fakeChatModel(answer))
	project, _ := s.database.GetActiveProjectID()

	// Nothing attached: no button, and a plain request for a photo.
	result := call(t, s, "POST", "/chat", `{"message":"make him dance"}`)
	if result.Code != 200 || !strings.Contains(result.Body.String(), `"action":"unknown"`) || !strings.Contains(result.Body.String(), "attach a photo") {
		t.Fatal(result.Code, result.Body.String())
	}

	if e := s.database.CreateAsset(&models.Asset{ID: "asset_photo", ProjectID: &project, Type: "other", Name: "man.jpg", FilePath: "storage/assets/asset_photo.jpg", TagsJSON: []byte("[]"), MetadataJSON: []byte("{}")}); e != nil {
		t.Fatal(e)
	}
	result = call(t, s, "POST", "/chat", `{"message":"make him dance","attachments":["asset_photo"]}`)
	var body struct {
		Intent struct {
			Action string
			Clip   struct {
				Prompt     string
				Duration   int
				Aspect     string `json:"aspect_ratio"`
				Audio      bool   `json:"generate_audio"`
				References []map[string]string
			}
		}
	}
	if result.Code != 200 || json.Unmarshal(result.Body.Bytes(), &body) != nil {
		t.Fatal(result.Code, result.Body.String())
	}
	clip := body.Intent.Clip
	if body.Intent.Action != "make_clip" || clip.Duration != 8 || clip.Aspect != "9:16" || !clip.Audio || len(clip.References) != 1 || clip.References[0]["asset_id"] != "asset_photo" || clip.References[0]["kind"] != "image" {
		t.Fatalf("%+v", body.Intent)
	}

	// Files from elsewhere are rejected rather than silently ignored.
	if result = call(t, s, "POST", "/chat", `{"message":"make him dance","attachments":["asset_missing"]}`); result.Code != 400 {
		t.Fatal(result.Code, result.Body.String())
	}
}

func TestTryAgainWithNoFinishedVideoBecomesMakingTheSceneVideo(t *testing.T) {
	answer := `{"reply":"I can redo it.","action":"retry_render","scene_id":"Not to Mourn"}`
	s := routerServer(t, fakeChatModel(answer))
	project, _ := s.database.GetActiveProjectID()
	for i, title := range []string{"The Quiet End", "Not to Mourn"} {
		if e := s.database.CreateScene(&models.Scene{ProjectID: &project, SceneOrder: i + 1, Title: title, Duration: 10, AspectRatio: "9:16", CharacterIDsJSON: []byte("[]"), SceneJSON: []byte("{}")}); e != nil {
			t.Fatal(e)
		}
	}
	w := call(t, s, "POST", "/chat", `{"message":"try again"}`)
	var body struct {
		Intent struct {
			Action  string
			SceneID string `json:"scene_id"`
		}
	}
	if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil || w.Code != 200 {
		t.Fatalf("chat: %d %s", w.Code, w.Body.String())
	}
	scenes, _ := s.database.ListScenes(project)
	want := ""
	for _, scene := range scenes {
		if scene.Title == "Not to Mourn" {
			want = scene.ID
		}
	}
	// With no finished video there is nothing to pick in an "Output" list, so the card
	// must be the one that makes the named scene's video.
	if body.Intent.Action != "render_scene" || body.Intent.SceneID != want {
		t.Fatalf("action = %q scene = %q, want render_scene on %q", body.Intent.Action, body.Intent.SceneID, want)
	}
}
