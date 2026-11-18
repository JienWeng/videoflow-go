package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestStoryboardVisualAudit(t *testing.T) {
	for _, pass := range []bool{true, false} {
		images, vision := 0, 0
		s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/models":
				io.WriteString(w, `{"data":[{"id":"openai/gpt-4o-mini","architecture":{"input_modalities":["text","image"],"output_modalities":["text"]}}]}`)
			case "/images/models":
				io.WriteString(w, `{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"aspect_ratio":{"type":"enum","values":["9:16"]}}}]}`)
			case "/images":
				images++
				io.WriteString(w, `{"data":[{"b64_json":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=","media_type":"image/png"}]}`)
			case "/chat/completions":
				data, _ := io.ReadAll(r.Body)
				content := `{"prompt":"Ocean keyframe"}`
				if strings.Contains(string(data), "data:image/png;base64,") {
					vision++
					content = `{"matches":false,"evidence":"Flat illustrated surface"}`
					if pass {
						content = `{"matches":true,"evidence":"Photographic texture"}`
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
			default:
				t.Errorf("unexpected call %s", r.URL.Path)
			}
		})
		var scene struct{ ID string }
		json.Unmarshal(call(t, s, "POST", "/scenes", `{"title":"Ocean","summary":"Rolling waves","duration":4,"aspect_ratio":"9:16"}`).Body.Bytes(), &scene)
		audit := func(body string) string {
			t.Helper()
			r := call(t, s, "POST", "/scenes/"+scene.ID+"/storyboard/audit", body)
			if r.Code != 200 {
				t.Fatal(r.Body.String())
			}
			var v struct{ Status string }
			json.Unmarshal(r.Body.Bytes(), &v)
			return v.Status
		}
		if audit(`{}`) != "needs_repair" || images != 0 || vision != 0 {
			t.Fatal("inspection spent credits on missing image")
		}
		status := audit(`{"repair":true}`)
		expected := "needs_attention"
		if pass {
			expected = "repaired"
		}
		if status != expected || images != 1 || vision != 1 {
			t.Fatal(status, images, vision)
		}
		if pass && audit(`{"repair":true}`) != "matched" {
			t.Fatal("matching board regenerated")
		}
		if !pass && audit(`{}`) != "needs_repair" {
			t.Fatal("rejected candidate attached")
		}
		if images != 1 {
			t.Fatal("unexpected second generation")
		}
	}
}
