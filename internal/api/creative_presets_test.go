package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"videoflow-go/internal/models"
	"videoflow-go/internal/presets"
)

func TestCreativePresetPackLifecycle(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected provider call %s", r.URL.Path)
		w.WriteHeader(500)
	})
	pack := presets.Builtins()
	pack.ID = "community-test"
	raw, _ := json.Marshal(pack)
	if r := call(t, s, "POST", "/preset-packs/preview", string(raw)); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	packs, _, _ := s.presetPacks()
	if len(packs) != 0 {
		t.Fatal("preview installed pack")
	}
	if r := call(t, s, "POST", "/preset-packs/import", string(raw)); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	r := call(t, s, "GET", "/style-presets", "")
	var catalog struct{ Presets []presets.Preset }
	json.Unmarshal(r.Body.Bytes(), &catalog)
	if len(catalog.Presets) != 2*len(pack.Presets) {
		t.Fatal(r.Body.String())
	}
	project, _ := s.database.GetActiveProjectID()
	s.database.UpsertStyleGuide(&models.StyleGuide{ProjectID: &project, Name: "Project", StylePrompt: "Project identity"})
	b := generationBrief{Idea: "A dog finds mother", PresetID: "community:community-test:clay-stop-motion", CreativePreset: &presets.Preset{Style: "UNTRUSTED"}}
	if err := s.prepareBrief(project, &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.Style, "clay") || b.CreativePreset == nil {
		t.Fatal("explicit preset was overridden", b)
	}
	frozen, _ := json.Marshal(b)
	pack.Presets[2].Style = "Changed style"
	changed, _ := json.Marshal(pack)
	if r = call(t, s, "POST", "/preset-packs/import", string(changed)); r.Code != 409 {
		t.Fatal("replacement lacked conflict", r.Code)
	}
	request := httptest.NewRequest("DELETE", "/preset-packs/community-test", nil)
	request.Header.Set("If-Match", packHash(presets.Pack{Format: pack.Format, SchemaVersion: pack.SchemaVersion, ID: pack.ID, Name: pack.Name, Version: pack.Version, Presets: presets.Builtins().Presets}))
	response := httptest.NewRecorder()
	s.router.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var saved generationBrief
	json.Unmarshal(frozen, &saved)
	if !strings.Contains(saved.CreativePreset.Style, "clay") {
		t.Fatal("snapshot lost")
	}
	if _, err := s.findPreset(b.PresetID); err == nil {
		t.Fatal("removed pack still available")
	}
	legacy := generationBrief{Idea: "A dog", Style: "cinematic"}
	if err := s.prepareBrief(project, &legacy); err != nil || !strings.Contains(legacy.Style, "Project identity") {
		t.Fatal("legacy project style changed", err)
	}
}
func TestSavedPresetDoesNotNeedInstalledPack(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	project, _ := s.database.GetActiveProjectID()
	snapshot := presets.Builtins().Presets[2]
	snapshot.ID = "community:removed:clay"
	b := generationBrief{Idea: "A dog", Style: snapshot.Style, PresetID: snapshot.ID, CreativePreset: &snapshot}
	if err := s.prepareSavedBrief(project, &b); err != nil {
		t.Fatal(err)
	}
	if b.CreativePreset != &snapshot || b.Style != snapshot.Style || b.PresetID != snapshot.ID {
		t.Fatal("saved preset changed")
	}
}
