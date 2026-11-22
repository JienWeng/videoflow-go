package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"videoflow-go/internal/models"
)

func TestProjectJSONRoundTripKeepsDataAndRemapsIDs(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	project, _ := s.database.GetActiveProjectID()
	c := models.Character{ProjectID: &project, Name: "Fox", Appearance: "Red coat"}
	if err := s.database.CreateCharacter(&c); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.cfg.StorageRoot, "reference.png"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	asset := models.Asset{ProjectID: &project, Name: "Reference", Type: "character_reference", CharacterID: &c.ID, FilePath: "storage/reference.png"}
	if err := s.database.CreateAsset(&asset); err != nil {
		t.Fatal(err)
	}
	c.ReferenceAssetIDsJSON, _ = json.Marshal([]string{asset.ID})
	if err := s.database.UpdateCharacter(&c); err != nil {
		t.Fatal(err)
	}
	scene := models.Scene{ProjectID: &project, Title: "Opening", CharacterIDsJSON: json.RawMessage(`["` + c.ID + `"]`)}
	if err := s.database.CreateScene(&scene); err != nil {
		t.Fatal(err)
	}
	shot := models.Shot{SceneID: scene.ID, Prompt: "Fox in a forest"}
	if err := s.database.CreateShot(&shot); err != nil {
		t.Fatal(err)
	}
	style := models.StyleGuide{ProjectID: &project, StylePrompt: "Ink illustration"}
	if err := s.database.UpsertStyleGuide(&style); err != nil {
		t.Fatal(err)
	}
	export := call(t, s, "GET", "/projects/"+project+"/export", "")
	if export.Code != 200 {
		t.Fatal(export.Code, export.Body.String())
	}
	imported := call(t, s, "POST", "/projects/import", export.Body.String())
	if imported.Code != 200 {
		t.Fatal(imported.Code, imported.Body.String())
	}
	var copy models.Project
	if err := json.Unmarshal(imported.Body.Bytes(), &copy); err != nil {
		t.Fatal(err)
	}
	chars, _ := s.database.ListCharacters(copy.ID)
	scenes, _ := s.database.ListScenes(copy.ID)
	if len(chars) != 1 || len(scenes) != 1 {
		t.Fatalf("import silently lost data: characters=%d scenes=%d", len(chars), len(scenes))
	}
	if chars[0].ID == c.ID || chars[0].Appearance != c.Appearance {
		t.Fatal("character identity was not copied safely")
	}
	assets, _ := s.database.ListAssets(copy.ID)
	if len(assets) != 1 || assets[0].CharacterID == nil || *assets[0].CharacterID != chars[0].ID {
		t.Fatal("asset binding lost", assets)
	}
	var references []string
	json.Unmarshal(chars[0].ReferenceAssetIDsJSON, &references)
	if len(references) != 1 || references[0] != assets[0].ID {
		t.Fatal("reference images not remapped", references)
	}
	var cast []string
	json.Unmarshal(scenes[0].CharacterIDsJSON, &cast)
	if len(cast) != 1 || cast[0] != chars[0].ID {
		t.Fatalf("cast references not remapped: %v", cast)
	}
	shots, _ := s.database.ListShots(scenes[0].ID)
	if len(shots) != 1 || shots[0].Prompt != shot.Prompt {
		t.Fatalf("shots lost: %v", shots)
	}
	copiedStyle, err := s.database.GetStyleGuide(copy.ID)
	if err != nil || copiedStyle.StylePrompt != style.StylePrompt {
		t.Fatal("style lost", err)
	}
	active, _ := s.database.GetActiveProjectID()
	if active != project {
		t.Fatal("import unexpectedly switched the active project")
	}
}

func TestProjectImportRollsBackWriteFailure(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	before, _ := s.database.ListProjects()
	response := call(t, s, "POST", "/projects/import", `{"project":{"name":"Broken"},"characters":[{"id":"fox","name":"Fox"}],"shots":[{"id":"shot","scene_id":"missing-scene"}]}`)
	if response.Code != 400 {
		t.Fatal("invalid shot accepted", response.Code, response.Body.String())
	}
	after, _ := s.database.ListProjects()
	if len(after) != len(before) {
		t.Fatal("failed import left partial project data")
	}
}

func TestProjectRenamePreservesDescription(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	p, err := s.database.CreateProject("Original", "Keep these notes")
	if err != nil {
		t.Fatal(err)
	}
	response := call(t, s, "PATCH", "/projects/"+p.ID, `{"name":"Renamed"}`)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	updated, _ := s.database.GetProject(p.ID)
	if updated.Name != "Renamed" || updated.Description != "Keep these notes" {
		t.Fatalf("rename lost project notes: %+v", updated)
	}
}

func TestInvalidProjectImportDoesNotCreateEmptyWorkspace(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	before, _ := s.database.ListProjects()
	response := call(t, s, "POST", "/projects/import", `{}`)
	if response.Code != 400 {
		t.Fatalf("expected invalid file rejection, got %d: %s", response.Code, response.Body.String())
	}
	after, _ := s.database.ListProjects()
	if len(after) != len(before) {
		t.Fatal("invalid import left a workspace behind")
	}
}
