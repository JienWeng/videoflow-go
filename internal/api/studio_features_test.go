package api

import (
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
func TestCaptionLocalizationKeepsIDsAndTiming(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("local translation escaped to remote text") })
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
