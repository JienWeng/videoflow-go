package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
	"videoflow-go/internal/models"
)

func TestChatCharacterReferenceActionAndFullContext(t *testing.T) {
	var prompt string
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
			return
		}
		raw := new(json.RawMessage)
		json.NewDecoder(r.Body).Decode(raw)
		prompt = string(*raw)
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"action\":\"generate_character_reference\",\"reply\":\"Review this reference request.\"}"}}]}`))
	})
	project, _ := s.database.GetActiveProjectID()
	c := &models.Character{ProjectID: &project, Name: "Mama", Appearance: "Golden fur, blue collar", VisualRulesJSON: json.RawMessage(`["Keep blue collar"]`)}
	s.database.CreateCharacter(c)
	r := call(t, s, "POST", "/chat", `{"message":"Generate Mama reference","selected":{"kind":"character","id":"`+c.ID+`"}}`)
	if r.Code != 200 || !strings.Contains(r.Body.String(), "generate_character_reference") {
		t.Fatal(r.Code, r.Body.String())
	}
	if !strings.Contains(prompt, "Golden fur") || !strings.Contains(prompt, "Keep blue collar") {
		t.Fatal("native planner lacks complete character context", prompt)
	}
}
func TestProjectStyleParticipatesInDirectorContextRevision(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no provider calls") })
	project, _ := s.database.GetActiveProjectID()
	first, _ := s.buildDirectorContext(project, directorSelection{})
	s.database.UpsertStyleGuide(&models.StyleGuide{ProjectID: &project, Name: "Clay", StylePrompt: "Handcrafted clay"})
	next, _ := s.buildDirectorContext(project, directorSelection{})
	if next.Style == nil || first.Revision == next.Revision {
		t.Fatal("style missing from frozen director context")
	}
}

func TestRemoteGuidedReviewNeverQueuesVideo(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
		case "/chat/completions":
			var body struct{ Messages []struct{ Content string } }
			json.NewDecoder(r.Body).Decode(&body)
			content := `{"prompt":"Ocean wave"}`
			if strings.Contains(body.Messages[0].Content, "scriptwriter") {
				content = `{"title":"Ocean","scenes":[{"title":"Wave","summary":"Ocean wave","duration":1,"aspect_ratio":"16:9"}]}`
			}
			if strings.Contains(body.Messages[0].Content, "cinematographer") {
				content = `{"shots":[{"shot_order":1,"duration":1,"prompt":"Ocean wave"}]}`
			}
			if asksForPlannedAssets(body.Messages[0].Content) {
				content = plannedAssetsAnswer
			}
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
		case "/images/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"aspect_ratio":{"type":"enum","values":["16:9"]}}}]}`))
		case "/images":
			w.Write([]byte(`{"data":[{"b64_json":"iVBORw0KGgo=","media_type":"image/png"}]}`))
		case "/videos/models":
			w.Write([]byte(`{"data":[{"id":"google/veo-3.1-lite","supported_durations":[1],"supported_aspect_ratios":["16:9"],"supported_frame_images":["first_frame"]}]}`))
		default:
			t.Errorf("unexpected provider call %s", r.URL.Path)
			w.WriteHeader(500)
		}
	})
	r := call(t, s, "POST", "/videos/generate", `{"idea":"Ocean wave","scene_count":1,"target_duration":1,"aspect_ratio":"16:9","review_before_render":true}`)
	if r.Code != 202 {
		t.Fatal(r.Code, r.Body.String())
	}
	var start struct {
		ID string `json:"op_id"`
	}
	json.Unmarshal(r.Body.Bytes(), &start)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		op, _ := s.database.GetOp(start.ID)
		if op.Status == "failed" {
			t.Fatal(*op.Error)
		}
		if op.Status == "succeeded" {
			if !strings.Contains(string(op.ResultJSON), "ready_to_render") {
				t.Fatal(string(op.ResultJSON))
			}
			project, _ := s.database.GetActiveProjectID()
			assets, _ := s.database.ListAssets(project)
			if len(assets) == 0 || !strings.Contains(string(assets[0].MetadataJSON), `"consistency_review":"ready"`) {
				t.Fatal("reviewable storyboards missing", assets)
			}
			jobs, _ := s.database.ListRenderJobs(project)
			if len(jobs) != 0 {
				t.Fatal("video queued without approval")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("operation did not finish")
}
func TestDirectImageRejectsWrongTypeAndMissingPromptBeforeProviders(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid request reached provider") })
	for _, body := range []string{`{}`, `{"prompt":"Puppy","type":"character_reference"}`, `{"prompt":"Puppy","aspect_ratio":"bad"}`, `{"prompt":"Puppy"} {}`} {
		r := call(t, s, "POST", "/assets/generate", body)
		if r.Code != 400 {
			t.Fatal(r.Code, r.Body.String())
		}
	}
}
func TestProjectStyleSaveRejectsProjectSwitch(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no provider calls") })
	original, _ := s.database.GetActiveProjectID()
	other, _ := s.database.CreateProject("Other", "")
	s.database.ActivateProject(other.ID)
	r := call(t, s, "PATCH", "/style", `{"project_id":"`+original+`","style_prompt":"Wrong project"}`)
	if r.Code != 409 {
		t.Fatal(r.Code, r.Body.String())
	}
	if style, err := s.database.GetStyleGuide(other.ID); err == nil && style.StylePrompt == "Wrong project" {
		t.Fatal("style leaked across projects")
	}
}
