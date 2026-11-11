package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"videoflow-go/internal/models"
)

func TestDirectorContextContainsFullSelectedIdentityAndDetectsReferenceChanges(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("context construction must not call AI") })
	project, _ := s.database.GetActiveProjectID()
	c := &models.Character{ProjectID: &project, Name: "Mimi", Description: "Family cat", Appearance: "white fur", Personality: "playful", VisualRulesJSON: json.RawMessage(`["green eyes"]`), VoiceRulesJSON: json.RawMessage(`["soft, low voice"]`), SampleDialogue: "Hello, I am Mimi"}
	if err := s.database.CreateCharacter(c); err != nil {
		t.Fatal(err)
	}
	photo := filepath.Join(s.cfg.StorageRoot, "mimi.png")
	os.WriteFile(photo, []byte("first reference"), 0600)
	portrait := &models.Asset{ProjectID: &project, CharacterID: &c.ID, Type: "character_reference", Name: "Mimi front", FilePath: photo}
	if err := s.database.CreateUploadedAsset(portrait); err != nil {
		t.Fatal(err)
	}
	carPath := filepath.Join(s.cfg.StorageRoot, "car.png")
	os.WriteFile(carPath, []byte("red car reference"), 0600)
	car := &models.Asset{ProjectID: &project, Type: "prop", Name: "Family car", Description: "Red passenger car", FilePath: carPath, MetadataJSON: json.RawMessage(`{"locked_traits":{"type":"car","color":"red"}}`)}
	if err := s.database.CreateAsset(car); err != nil {
		t.Fatal(err)
	}
	selected := directorSelection{CharacterIDs: []string{c.ID}, AssetIDs: []string{car.ID}}
	first, err := s.buildDirectorContext(project, selected)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(first)
	for _, expected := range []string{"white fur", "playful", "green eyes", "soft, low voice", "Hello, I am Mimi", portrait.ID, car.ID, `"color":"red"`, `"type":"car"`} {
		if !strings.Contains(string(raw), expected) {
			t.Errorf("context lost %q", expected)
		}
	}
	if strings.Contains(string(raw), "saved-key") {
		t.Fatal("context exposed provider credentials")
	}
	again, _ := s.buildDirectorContext(project, selected)
	if first.Revision == "" || first.Revision != again.Revision {
		t.Fatal("context revision is not stable")
	}
	os.WriteFile(carPath, []byte("changed reference file"), 0600)
	changed, _ := s.buildDirectorContext(project, selected)
	if changed.Revision == first.Revision {
		t.Fatal("reference file changed without invalidating context")
	}
}
func TestDirectorContextRejectsForeignSelection(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected provider call") })
	project, _ := s.database.GetActiveProjectID()
	other, err := s.database.CreateProject("Other", "")
	if err != nil {
		t.Fatal(err)
	}
	c := &models.Character{ProjectID: &other.ID, Name: "Foreign"}
	s.database.CreateCharacter(c)
	if _, err := s.buildDirectorContext(project, directorSelection{CharacterIDs: []string{c.ID}}); err == nil {
		t.Fatal("foreign character entered project context")
	}
	asset := &models.Asset{ProjectID: &other.ID, Name: "Foreign prop", FilePath: "storage/foreign.png"}
	s.database.CreateAsset(asset)
	if _, err := s.buildDirectorContext(project, directorSelection{AssetIDs: []string{asset.ID}}); err == nil {
		t.Fatal("foreign asset entered project context")
	}
}
func TestSelectedImageReferencesAndLockedTraitsReachGenerationInputs(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no inference") })
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	path := filepath.Join(s.cfg.StorageRoot, "red.png")
	photo, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aO1cAAAAASUVORK5CYII=")
	os.WriteFile(path, photo, 0600)
	asset := &models.Asset{ProjectID: &project, Name: "Red car", Type: "prop", FilePath: path, MetadataJSON: []byte(`{"locked_traits":{"kind":"car","color":"red"}}`)}
	s.database.CreateAsset(asset)
	ctx := context.WithValue(context.Background(), directorBindingKey{}, directorBinding{ProjectID: project, Selection: directorSelection{AssetIDs: []string{asset.ID}}})
	refs, notes, err := s.directorImageInputs(ctx, &project, nil)
	if err != nil || len(refs) != 1 || !strings.HasPrefix(refs[0], "data:image/png") || !strings.Contains(notes, `"color":"red"`) {
		t.Fatalf("refs %d notes %s err %v", len(refs), notes, err)
	}
	os.Remove(path)
	if _, _, err = s.directorImageInputs(ctx, &project, nil); err == nil {
		t.Fatal("missing approved reference silently dropped")
	}
}
func TestAssetIdentityLocksRequireCurrentProjectAndRevision(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no model needed") })
	project, _ := s.database.GetActiveProjectID()
	asset := &models.Asset{ProjectID: &project, Name: "Car", Type: "prop", FilePath: "storage/car.png"}
	s.database.CreateAsset(asset)
	payload := fmt.Sprintf(`{"expected_updated_at":%q,"locked_traits":{"kind":"car","color":"red"}}`, asset.UpdatedAt.Format(time.RFC3339Nano))
	saved := call(t, s, "PUT", "/assets/"+asset.ID+"/identity", payload)
	if saved.Code != 200 {
		t.Fatalf("%d %s", saved.Code, saved.Body.String())
	}
	again := call(t, s, "PUT", "/assets/"+asset.ID+"/identity", payload)
	if again.Code != 409 {
		t.Fatal("stale traits overwrote current identity", again.Code)
	}
}
func TestRecognitionPreservesIdentityLocksEditedDuringVisionCall(t *testing.T) {
	var server *Server
	var assetID string
	server = routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"qwen/qwen3-vl-30b-a3b-instruct","architecture":{"input_modalities":["text","image"],"output_modalities":["text"]}}]}`))
			return
		}
		current, _ := server.database.GetAsset(assetID)
		current.MetadataJSON = []byte(`{"locked_traits":{"color":"red","kind":"car"}}`)
		server.database.UpdateAsset(current)
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"description\":\"A red car\",\"tags\":[\"car\"]}"}}]}`))
	})
	project, _ := server.database.GetActiveProjectID()
	path := filepath.Join(server.cfg.StorageRoot, "ref.png")
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aO1cAAAAASUVORK5CYII=")
	os.WriteFile(path, png, 0600)
	asset := &models.Asset{ProjectID: &project, Name: "Car", Type: "prop", FilePath: path}
	server.database.CreateAsset(asset)
	assetID = asset.ID
	response := call(t, server, "POST", "/assets/"+assetID+"/recognise", `{"description":"Describe this car"}`)
	if response.Code != 200 {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
	current, _ := server.database.GetAsset(assetID)
	if !strings.Contains(string(current.MetadataJSON), `"color":"red"`) {
		t.Fatal("vision call erased concurrent approved locks", string(current.MetadataJSON))
	}
}
