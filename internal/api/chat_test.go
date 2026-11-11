package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
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
		sawContext = strings.Contains(string(raw), "selected_item") && strings.Contains(string(raw), "crimson car")
		content := `{"reply":"Refine selected scene","action":"refine_scene","scene_id":"wrong-scene"}`
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	})
	created := call(t, s, "POST", "/scenes", `{"title":"Car scene","summary":"crimson car"}`)
	var scene struct {
		ID string `json:"id"`
	}
	json.Unmarshal(created.Body.Bytes(), &scene)
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
