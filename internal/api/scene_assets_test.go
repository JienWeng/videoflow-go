package api

import (
	"context"
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

func TestSceneGenerationUsesOnlyItsSavedReferences(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("no provider call") })
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	path := filepath.Join(s.cfg.StorageRoot, "prop.png")
	os.WriteFile(path, []byte{137, 80, 78, 71, 13, 10, 26, 10}, 0600)
	a := &models.Asset{ProjectID: &project, Type: "prop", Name: "Unrelated car", FilePath: "storage/prop.png"}
	s.database.CreateAsset(a)
	scene := &models.Scene{ProjectID: &project, Title: "Dog at home"}
	s.database.CreateScene(scene)
	ctx := context.WithValue(context.Background(), directorBindingKey{}, directorBinding{ProjectID: project, Selection: directorSelection{AssetIDs: []string{a.ID}}})
	_, snapshot, err := s.directorGenerationContext(ctx, &project, scene.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Assets) != 0 {
		t.Fatal("unrelated chat selection leaked into scene", snapshot.Assets)
	}
}

func TestSceneRenderDoesNotUsePropAsKeyframe(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("must reject before provider") })
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	path := filepath.Join(s.cfg.StorageRoot, "prop.png")
	os.WriteFile(path, []byte{137, 80, 78, 71, 13, 10, 26, 10}, 0600)
	a := &models.Asset{ProjectID: &project, Type: "prop", Name: "Collar", FilePath: "storage/prop.png"}
	s.database.CreateAsset(a)
	scene := &models.Scene{ProjectID: &project, Title: "Home", SceneJSON: []byte(`{"story_asset_ids":["` + a.ID + `"]}`)}
	s.database.CreateScene(scene)
	ctx := guidedStoryContext(context.Background(), project, nil)
	job := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, RequestJSON: []byte(`{"prompt":"Scene"}`)}
	if err := s.freezeDirectorVideo(ctx, job); err == nil {
		t.Fatal("prop substituted for a scene keyframe")
	}
}

func TestSharedSceneReferenceLifecycle(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("no paid calls") })
	project, _ := s.database.GetActiveProjectID()
	os.WriteFile(filepath.Join(s.cfg.StorageRoot, "collar.png"), []byte{137, 80, 78, 71, 13, 10, 26, 10}, 0600)
	a := &models.Asset{ProjectID: &project, Type: "prop", Name: "Collar", FilePath: "storage/collar.png", MetadataJSON: []byte(`{"consistency_review":"approved","locked_traits":{"color":"red"}}`)}
	s.database.CreateAsset(a)
	scenes := []models.Scene{{ProjectID: &project, Title: "Forest", SceneJSON: []byte(`{"keep":true}`)}, {ProjectID: &project, Title: "Home", SceneJSON: []byte(`{"keep":true}`)}}
	for i := range scenes {
		s.database.CreateScene(&scenes[i])
		body := fmt.Sprintf(`{"expected_updated_at":%q}`, scenes[i].UpdatedAt.Format(time.RFC3339Nano))
		response := call(t, s, "POST", "/scenes/"+scenes[i].ID+"/references/"+a.ID, body)
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body.String())
		}
		if stale := call(t, s, "POST", "/scenes/"+scenes[i].ID+"/references/"+a.ID, body); stale.Code != 409 {
			t.Fatal("stale attachment accepted", stale.Code)
		}
	}
	usage := call(t, s, "GET", "/assets/"+a.ID+"/usage", "")
	if usage.Code != 200 || !strings.Contains(usage.Body.String(), "Forest") || !strings.Contains(usage.Body.String(), "Home") {
		t.Fatal(usage.Code, usage.Body.String())
	}
	if deletion := call(t, s, "DELETE", "/assets/"+a.ID, ""); deletion.Code != 409 {
		t.Fatal("shared asset deleted", deletion.Code, deletion.Body.String())
	}
	for _, scene := range scenes {
		saved, _ := s.database.GetScene(scene.ID)
		body := fmt.Sprintf(`{"expected_updated_at":%q}`, saved.UpdatedAt.Format(time.RFC3339Nano))
		response := call(t, s, "DELETE", "/scenes/"+scene.ID+"/references/"+a.ID, body)
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body.String())
		}
		saved, _ = s.database.GetScene(scene.ID)
		if !strings.Contains(string(saved.SceneJSON), `"keep":true`) {
			t.Fatal("scene content overwritten")
		}
	}
	if deletion := call(t, s, "DELETE", "/assets/"+a.ID, ""); deletion.Code != 200 {
		t.Fatal(deletion.Code, deletion.Body.String())
	}
}

func TestSceneReferenceOwnershipAndRejectedImage(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("no paid calls") })
	project, _ := s.database.GetActiveProjectID()
	other, _ := s.database.CreateProject("Other", "")
	foreign := &models.Asset{ProjectID: &other.ID, Type: "prop", Name: "Other prop"}
	s.database.CreateAsset(foreign)
	scene := &models.Scene{ProjectID: &project, Title: "Home"}
	s.database.CreateScene(scene)
	body := fmt.Sprintf(`{"expected_updated_at":%q}`, scene.UpdatedAt.Format(time.RFC3339Nano))
	for _, method := range []string{"GET", "DELETE"} {
		response := call(t, s, method, "/assets/"+foreign.ID, "")
		if response.Code != 404 {
			t.Fatal("foreign asset exposed", method, response.Code)
		}
	}
	if response := call(t, s, "POST", "/scenes/"+scene.ID+"/references/"+foreign.ID, body); response.Code != 404 {
		t.Fatal("foreign reference attached", response.Code)
	}
	rejected := &models.Asset{ProjectID: &project, Type: "prop", Name: "Rejected", MetadataJSON: []byte(`{"consistency_review":"rejected"}`)}
	s.database.CreateAsset(rejected)
	if response := call(t, s, "POST", "/scenes/"+scene.ID+"/references/"+rejected.ID, body); response.Code != 409 {
		t.Fatal("rejected reference attached", response.Code)
	}
}

func TestStylePatchPreservesProjectStyle(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("no paid calls") })
	project, _ := s.database.GetActiveProjectID()
	style := &models.StyleGuide{ProjectID: &project, Name: "Picture book", StylePrompt: "Warm watercolor", Palette: "Pastel", Tone: "Kind"}
	s.database.UpsertStyleGuide(style)
	response := call(t, s, "PATCH", "/style", `{"tone":"Playful"}`)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	saved, _ := s.database.GetStyleGuide(project)
	if saved.Tone != "Playful" || saved.StylePrompt != "Warm watercolor" || saved.Palette != "Pastel" || saved.ID != style.ID {
		t.Fatal("partial style edit lost the project identity", saved)
	}
}

func TestAssetVersionPreservesOldScenesAndLocks(t *testing.T) {
	images := 0
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
		case "/chat/completions":
			w.Write([]byte(`{"choices":[{"message":{"content":"{\"prompt\":\"Red collar\"}"}}]}`))
		case "/images/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"input_references":{"type":"array"},"aspect_ratio":{"type":"enum","values":["1:1"]}}}]}`))
		case "/images":
			images++
			w.Write([]byte(`{"data":[{"b64_json":"iVBORw0KGgo=","media_type":"image/png"}]}`))
		default:
			t.Errorf("unexpected provider %s", r.URL.Path)
			w.WriteHeader(500)
		}
	})
	project, _ := s.database.GetActiveProjectID()
	os.WriteFile(filepath.Join(s.cfg.StorageRoot, "red.png"), []byte{137, 80, 78, 71, 13, 10, 26, 10}, 0600)
	a := &models.Asset{ProjectID: &project, Type: "prop", Name: "Collar", Description: "Red leather collar", FilePath: "storage/red.png", MetadataJSON: []byte(`{"consistency_review":"approved","locked_traits":{"color":"red"}}`)}
	s.database.CreateAsset(a)
	scene := &models.Scene{ProjectID: &project, Title: "Home", AssetIDsJSON: []byte(`["` + a.ID + `"]`), SceneJSON: []byte(`{"story_asset_ids":["` + a.ID + `"]}`)}
	s.database.CreateScene(scene)
	body := fmt.Sprintf(`{"expected_updated_at":%q}`, a.UpdatedAt.Format(time.RFC3339Nano))
	response := call(t, s, "POST", "/assets/"+a.ID+"/regenerate", body)
	if response.Code != 200 || images != 1 {
		t.Fatal(response.Code, response.Body.String(), images)
	}
	var result struct {
		IDs []string `json:"asset_ids"`
	}
	json.Unmarshal(response.Body.Bytes(), &result)
	if len(result.IDs) != 1 {
		t.Fatal(response.Body.String())
	}
	next, _ := s.database.GetAsset(result.IDs[0])
	var metadata map[string]any
	json.Unmarshal(next.MetadataJSON, &metadata)
	if metadata["source_asset_id"] != a.ID || metadata["variant_group_id"] != a.ID || metadata["consistency_review"] != "pending" || metadata["locked_traits"].(map[string]any)["color"] != "red" {
		t.Fatal(metadata)
	}
	saved, _ := s.database.GetScene(scene.ID)
	if ids := sceneStoryAssetIDs(saved); len(ids) != 1 || ids[0] != a.ID {
		t.Fatal("regeneration changed existing scene")
	}
	old, _ := s.database.GetAsset(a.ID)
	if !old.UpdatedAt.Equal(a.UpdatedAt) {
		t.Fatal("old identity mutated")
	}
	replace := fmt.Sprintf(`{"expected_updated_at":%q,"replace_asset_id":%q}`, saved.UpdatedAt.Format(time.RFC3339Nano), a.ID)
	response = call(t, s, "POST", "/scenes/"+scene.ID+"/references/"+next.ID, replace)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	saved, _ = s.database.GetScene(scene.ID)
	if ids := sceneStoryAssetIDs(saved); len(ids) != 1 || ids[0] != next.ID {
		t.Fatal("version replacement duplicated identities", ids)
	}
}

func TestAssetDetailsEditPreservesLocksAndRequiresReview(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("no paid calls") })
	project, _ := s.database.GetActiveProjectID()
	a := &models.Asset{ProjectID: &project, Type: "prop", Name: "Collar", Description: "Red", MetadataJSON: []byte(`{"consistency_review":"approved","locked_traits":{"color":"red"},"keep":true}`)}
	s.database.CreateAsset(a)
	response := call(t, s, "PATCH", "/assets/"+a.ID, fmt.Sprintf(`{"expected_updated_at":%q,"name":"Red collar","description":"Small red leather collar","locked_traits":{"color":"red"}}`, a.UpdatedAt.Format(time.RFC3339Nano)))
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	saved, _ := s.database.GetAsset(a.ID)
	if saved.Description != "Small red leather collar" || !strings.Contains(string(saved.MetadataJSON), `"consistency_review":"pending"`) || !strings.Contains(string(saved.MetadataJSON), `"keep":true`) {
		t.Fatal("identity or review lost", saved)
	}
}

func TestSceneRenderUsesNewestReviewedMatchingStoryboard(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/videos/models" {
			w.Write([]byte(`{"data":[{"id":"google/veo-3.1-lite","supported_durations":[1],"supported_aspect_ratios":["16:9"],"supported_frame_images":["first_frame"]}]}`))
			return
		}
		t.Fatalf("unexpected provider %s", r.URL.Path)
	})
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	scene := &models.Scene{ProjectID: &project, Title: "Home"}
	s.database.CreateScene(scene)
	ctx := guidedStoryContext(context.Background(), project, nil)
	_, snapshot, err := s.directorGenerationContext(ctx, &project, scene.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, name := range []string{"old", "new"} {
		path := filepath.Join(s.cfg.StorageRoot, name+".png")
		os.WriteFile(path, []byte{137, 80, 78, 71, 13, 10, 26, 10}, 0600)
		meta, _ := json.Marshal(map[string]any{"scene_id": scene.ID, "consistency_review": "approved", "director_context_revision": snapshot.Revision})
		a := &models.Asset{ProjectID: &project, Type: "storyboard", Name: name, FilePath: "storage/" + name + ".png", MetadataJSON: meta}
		s.database.CreateAsset(a)
		ids = append(ids, a.ID)
	}
	scene.AssetIDsJSON, _ = json.Marshal(ids)
	s.database.UpdateScene(scene)
	job := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Provider: "openrouter", Model: "google/veo-3.1-lite", RequestJSON: []byte(`{"prompt":"Scene"}`)}
	if err = s.freezeDirectorVideo(ctx, job); err != nil {
		t.Fatal(err)
	}
	var request map[string]any
	json.Unmarshal(job.RequestJSON, &request)
	if request["director_keyframe"] != ids[1] {
		t.Fatal("render used older storyboard", request["director_keyframe"])
	}
}

func TestSceneContentChangesInvalidateReviewedStoryboardAndQueuedRender(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/videos/models" {
			w.Write([]byte(`{"data":[{"id":"google/veo-3.1-lite","supported_durations":[1],"supported_aspect_ratios":["16:9"],"supported_frame_images":["first_frame"]}]}`))
			return
		}
		t.Fatal("no image or video generation")
	})
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	scene := &models.Scene{ProjectID: &project, Title: "Home", Summary: "Puppy arrives", Duration: 1, AspectRatio: "16:9"}
	s.database.CreateScene(scene)
	revision, _ := s.sceneInputRevision(scene)
	os.WriteFile(filepath.Join(s.cfg.StorageRoot, "board.png"), []byte{137, 80, 78, 71, 13, 10, 26, 10}, 0600)
	meta, _ := json.Marshal(map[string]any{"scene_id": scene.ID, "scene_input_revision": revision, "consistency_review": "approved"})
	a := &models.Asset{ProjectID: &project, Type: "storyboard", Name: "Home", FilePath: "storage/board.png", MetadataJSON: meta}
	s.database.CreateAsset(a)
	scene.AssetIDsJSON, _ = json.Marshal([]string{a.ID})
	s.database.UpdateScene(scene)
	ctx := guidedStoryContext(context.Background(), project, nil)
	job := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Provider: "openrouter", Model: "google/veo-3.1-lite", RequestJSON: []byte(`{"prompt":"Home"}`)}
	if err := s.freezeDirectorVideo(ctx, job); err != nil {
		t.Fatal(err)
	}
	scene.Summary = "Puppy leaves"
	s.database.UpdateScene(scene)
	state, err := s.sceneReferenceState(ctx, scene)
	if err != nil || state["storyboard_ready"] != false {
		t.Fatal("old storyboard reported current", state, err)
	}
	if err = s.validateFrozenVideo(ctx, job); err == nil {
		t.Fatal("queued render ignored scene content edit")
	}
	fresh := &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, RequestJSON: []byte(`{"prompt":"Home"}`)}
	if err = s.freezeDirectorVideo(ctx, fresh); err == nil {
		t.Fatal("old keyframe accepted after story edit")
	}
	a, _ = s.database.GetAsset(a.ID)
	response := call(t, s, "POST", "/assets/"+a.ID+"/review", fmt.Sprintf(`{"approved":true,"expected_updated_at":%q}`, a.UpdatedAt.Format(time.RFC3339Nano)))
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	state, err = s.sceneReferenceState(ctx, scene)
	if err != nil || state["storyboard_ready"] != true {
		t.Fatal("explicit current-scene review not respected", state, err)
	}
	before, _ := s.sceneInputRevision(scene)
	s.database.UpsertStyleGuide(&models.StyleGuide{ProjectID: &project, StylePrompt: "Ink drawing"})
	after, _ := s.sceneInputRevision(scene)
	if before == after {
		t.Fatal("project visual style did not invalidate scene input revision")
	}
}
