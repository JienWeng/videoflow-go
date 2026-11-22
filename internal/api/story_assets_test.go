package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"videoflow-go/internal/models"
)

func TestMatchingStoryAssetPreservesAppearance(t *testing.T) {
	item := plannedAsset{Name: "Red collar", Type: "prop", Description: "A small red leather collar"}
	assets := []models.Asset{{ID: "blue", Type: "prop", Name: "Red collar", Description: "A blue collar"}, {ID: "red", Type: "prop", Name: " Red COLLAR ", Description: "A small red leather collar"}}
	if got := matchingStoryAsset(item, assets); got != "red" {
		t.Fatalf("wanted exact appearance match, got %q", got)
	}
	item.Description = "A green collar"
	if got := matchingStoryAsset(item, assets); got != "" {
		t.Fatalf("must not reuse a different appearance: %s", got)
	}
}

func TestStoryAssetReuseWithoutProviderAndRejectsForeignID(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("reuse must not call provider") })
	project, _ := s.database.GetActiveProjectID()
	path := filepath.Join(s.cfg.StorageRoot, "collar.png")
	os.WriteFile(path, []byte{137, 80, 78, 71, 13, 10, 26, 10}, 0600)
	asset := &models.Asset{ProjectID: &project, Type: "prop", Name: "Collar", Description: "Red leather", FilePath: "storage/collar.png", MetadataJSON: []byte(`{"locked_traits":{"color":"red"}}`)}
	if err := s.database.CreateAsset(asset); err != nil {
		t.Fatal(err)
	}
	scene := &models.Scene{ProjectID: &project, Title: "Home", SceneJSON: []byte(`{"spoken_lines":[],"keep":"value"}`)}
	if err := s.database.CreateScene(scene); err != nil {
		t.Fatal(err)
	}
	plan := &assetPlan{Assets: []plannedAsset{{Name: asset.Name, Description: asset.Description, Type: "prop"}}}
	for i := 0; i < 2; i++ {
		got, err := s.materializeStoryAssets(context.Background(), scene, plan)
		if err != nil || len(got) != 1 || got[0].ID != asset.ID {
			t.Fatalf("reuse: %v %v", got, err)
		}
	}
	saved, _ := s.database.GetScene(scene.ID)
	if ids := sceneStoryAssetIDs(saved); len(ids) != 1 || ids[0] != asset.ID {
		t.Fatalf("duplicate or missing links: %v", ids)
	}
	if !strings.Contains(string(saved.SceneJSON), `"keep":"value"`) {
		t.Fatal("lost scene metadata")
	}
	plan.Assets[0].ExistingAssetID = "foreign-asset"
	if _, err := s.materializeStoryAssets(context.Background(), scene, plan); err == nil {
		t.Fatal("invented/foreign reuse accepted")
	}
	unchanged, _ := s.database.GetAsset(asset.ID)
	if string(unchanged.MetadataJSON) != string(asset.MetadataJSON) {
		t.Fatal("reuse changed locked identity")
	}
}

func TestExcludeStoryAssetRetainsLibraryAndFencesContinuation(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("no paid calls") })
	s.router.Post("/videos/generate/{id}/assets/exclude", s.handleExcludeStoryAsset)
	project, _ := s.database.GetActiveProjectID()
	asset := &models.Asset{ProjectID: &project, Type: "prop", Name: "Collar"}
	s.database.CreateAsset(asset)
	scene := &models.Scene{ProjectID: &project, Title: "Home", SceneJSON: []byte(`{"story_asset_ids":["` + asset.ID + `"],"keep":true}`), AssetIDsJSON: []byte(`["` + asset.ID + `"]`)}
	s.database.CreateScene(scene)
	state := storyContinuation{ProjectID: project, SceneIDs: []string{scene.ID}, AssetReferences: []string{asset.ID}, Status: "awaiting_character_review"}
	raw, _ := json.Marshal(state)
	op := &models.Op{ProjectID: &project, Kind: "video_generation", Status: "succeeded", ResultJSON: raw}
	s.database.CreateOp(op)
	response := call(t, s, "POST", "/videos/generate/"+op.ID+"/assets/exclude", `{"asset_id":"`+asset.ID+`"}`)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	saved, _ := s.database.GetScene(scene.ID)
	if len(sceneStoryAssetIDs(saved)) != 0 || !strings.Contains(string(saved.SceneJSON), `"keep":true`) {
		t.Fatal("scene bindings not safely removed", string(saved.SceneJSON))
	}
	if _, err := s.database.GetAsset(asset.ID); err != nil {
		t.Fatal("library image deleted", err)
	}
	claimed, err := s.database.ClaimStoryContinuation(op.ID, project, raw)
	if err != nil || claimed {
		t.Fatal("stale continuation passed", err)
	}
	other, _ := s.database.CreateProject("Other", "")
	s.database.ActivateProject(other.ID)
	if response = call(t, s, "POST", "/videos/generate/"+op.ID+"/assets/exclude", `{"asset_id":"`+asset.ID+`"}`); response.Code != 404 {
		t.Fatal("cross-project exclusion", response.Code)
	}
}

func TestMissingStoryAssetGeneratedOnceAcrossScenes(t *testing.T) {
	images := 0
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
		case "/images/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"aspect_ratio":{"type":"enum","values":["1:1"]}}}]}`))
		case "/chat/completions":
			w.Write([]byte(`{"choices":[{"message":{"content":"{\"prompt\":\"A red leather collar\"}"}}]}`))
		case "/images":
			images++
			w.Write([]byte(`{"data":[{"b64_json":"iVBORw0KGgo=","media_type":"image/png"}]}`))
		default:
			t.Errorf("unexpected provider request %s", r.URL.Path)
			w.WriteHeader(500)
		}
	})
	project, _ := s.database.GetActiveProjectID()
	var first string
	for i := 0; i < 2; i++ {
		scene := &models.Scene{ProjectID: &project, Title: "Collar scene"}
		s.database.CreateScene(scene)
		plan := &assetPlan{Assets: []plannedAsset{{Type: "prop", Name: "Collar", Description: "Red leather collar"}}}
		got, err := s.materializeStoryAssets(plannerContext(t, s), scene, plan)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Fatal("missing generated reference")
		}
		if i == 0 {
			first = got[0].ID
		} else if got[0].ID != first || !plan.Assets[0].Reuse {
			t.Fatal("cross-scene duplicate generated")
		}
	}
	if images != 1 {
		t.Fatalf("expected one paid image, got %d", images)
	}
}
