package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"videoflow-go/internal/models"
)

func TestStudioBrandCASAndProjectScope(t *testing.T) {
	s := routerServer(t, nil)
	current := call(t, s, "GET", "/studio/brand", "")
	var brand studioBrand
	json.Unmarshal(current.Body.Bytes(), &brand)
	brand.Name = "Pawprints"
	body := string(voiceJSON(brand))
	if r := call(t, s, "PUT", "/studio/brand", body); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if r := call(t, s, "PUT", "/studio/brand", body); r.Code != 409 {
		t.Fatal("stale brand overwrote changes")
	}
	other, _ := s.database.CreateProject("Other", "")
	asset := &models.Asset{ProjectID: &other.ID, Name: "Foreign", Type: "other", FilePath: "storage/no.png"}
	s.database.CreateAsset(asset)
	current = call(t, s, "GET", "/studio/brand", "")
	json.Unmarshal(current.Body.Bytes(), &brand)
	brand.Logo = asset.ID
	if r := call(t, s, "PUT", "/studio/brand", string(voiceJSON(brand))); r.Code != 400 {
		t.Fatal("foreign logo accepted")
	}
}
func TestReferenceVideoPlanConfirmAndDeduplicate(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected provider call %s", r.URL)
		w.WriteHeader(500)
	})
	project, _ := s.database.GetActiveProjectID()
	s.cfg.FalAPIKey = "fixture-key"
	s.cfg.FalBaseURL = s.cfg.OpenRouterBaseURL
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	os.WriteFile(filepath.Join(s.cfg.StorageRoot, "look.png"), data, 0600)
	asset := &models.Asset{ProjectID: &project, Type: "other", Name: "Look", FilePath: "storage/look.png"}
	s.database.CreateAsset(asset)
	request := referenceVideoRequest{Prompt: "Puppy waves", Duration: 8, Aspect: "9:16", Resolution: "480p", References: []generationReference{{AssetID: asset.ID, Kind: "image", Purpose: "Appearance"}}}
	response := call(t, s, "POST", "/reference-videos/plan", string(voiceJSON(request)))
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var plan struct {
		ID string `json:"id"`
	}
	json.Unmarshal(response.Body.Bytes(), &plan)
	request.Confirmed = "stale"
	if r := call(t, s, "POST", "/reference-videos/render", string(voiceJSON(request))); r.Code != 409 {
		t.Fatal("changed plan accepted", r.Body.String())
	}
	request.Confirmed = plan.ID
	for i := 0; i < 2; i++ {
		r := call(t, s, "POST", "/reference-videos/render", string(voiceJSON(request)))
		if r.Code != 202 {
			t.Fatal(r.Code, r.Body.String())
		}
	}
	jobs, _ := s.database.ListRenderJobs(project)
	if len(jobs) != 1 {
		t.Fatal("duplicate generation queued", len(jobs))
	}
	if !strings.Contains(string(jobs[0].RequestJSON), "image_urls") {
		t.Fatal("lost actual reference input")
	}
}
func TestCaptionLocalizationKeepsIDsAndTiming(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("local translation escaped to remote text") })
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	s.database.CreateDirectorSession(project, "codex", "Editing")
	fake := filepath.Join(t.TempDir(), "codex")
	script := `#!/bin/sh
cat >/dev/null
printf '%s\n' '{"type":"thread.started","thread_id":"locale"}' '{"type":"item.completed","item":{"type":"agent_message","text":"{\"translations\":[{\"id\":\"cue-1\",\"text\":\"Hola\"}]}"}}' '{"type":"turn.completed"}'
`
	os.WriteFile(fake, []byte(script), 0700)
	s.directorRunner.Codex = fake
	r := call(t, s, "POST", "/compositions/localize", `{"language":"Spanish","captions":[{"id":"cue-1","text":"Hello","start":0.2,"end":1.2}]}`)
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"text":"Hola"`) || !strings.Contains(r.Body.String(), `"start":0.2`) || !strings.Contains(r.Body.String(), `"audio_changed":false`) {
		t.Fatal(r.Code, r.Body.String())
	}
}
