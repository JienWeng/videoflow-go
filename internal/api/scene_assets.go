package api

import (
	"context"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"time"
	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
)

func (s *Server) sceneReferenceState(ctx context.Context, scene *models.Scene) (map[string]any, error) {
	cast := models.IDList(scene.CharacterIDsJSON)
	ids := append([]string{}, s.sceneBoundAssets(scene)...)
	missingCharacters := []string{}
	for _, id := range cast {
		c, e := s.database.GetCharacter(id)
		if e != nil || c.ProjectID == nil || *c.ProjectID != *scene.ProjectID {
			missingCharacters = append(missingCharacters, id)
			continue
		}
		refs := models.IDList(c.ReferenceAssetIDsJSON)
		if len(refs) == 0 {
			missingCharacters = append(missingCharacters, id)
		}
		ids = append(ids, refs...)
	}
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	assets, err := s.database.ListAssets(*scene.ProjectID)
	if err != nil {
		return nil, err
	}
	references, boards := []models.Asset{}, []models.Asset{}
	missing := []string{}
	for _, a := range assets {
		if selected[a.ID] {
			references = append(references, a)
			delete(selected, a.ID)
		}
		var metadata struct {
			Scene string `json:"scene_id"`
		}
		_ = json.Unmarshal(a.MetadataJSON, &metadata) // unreadable metadata belongs to no scene
		if a.Type == "storyboard" && metadata.Scene == scene.ID {
			boards = append(boards, a)
		}
	}
	for id := range selected {
		missing = append(missing, id)
	}
	current := ""
	if _, snapshot, e := s.directorGenerationContext(guidedStoryContext(ctx, *scene.ProjectID, nil), scene.ProjectID, scene.ID, nil); e == nil {
		current = snapshot.Revision
	}
	ready := len(missing) == 0 && len(missingCharacters) == 0 && len(references) <= 8
	for i := range references {
		a := &references[i]
		if !models.ReferenceApproved(a.MetadataJSON) {
			ready = false
		}
		if _, e := s.imageURL(a); e != nil {
			ready = false
		}
	}
	inputRevision, e := s.sceneInputRevision(scene)
	if e != nil {
		return nil, e
	}
	boardReady := false
	activeBoard := ""
	active := models.IDList(scene.AssetIDsJSON)
	activeIDs := map[string]bool{}
	for _, id := range active {
		activeIDs[id] = true
	}
	for i := range boards {
		a := &boards[i]
		if !ready || !activeIDs[a.ID] || !models.ReferenceApproved(a.MetadataJSON) {
			continue
		}
		var metadata struct {
			Revision string `json:"director_context_revision"`
			Inputs   string `json:"scene_input_revision"`
		}
		_ = json.Unmarshal(a.MetadataJSON, &metadata) // unreadable metadata has no revisions
		if metadata.Inputs != "" && metadata.Inputs != inputRevision {
			continue
		}
		if metadata.Revision != current && !(metadata.Revision == "" && len(references) == 0) {
			continue
		}
		if _, e := s.imageURL(a); e == nil {
			boardReady = true
			if activeBoard == "" {
				activeBoard = a.ID
			}
		}
	}
	return map[string]any{"scene_id": scene.ID, "title": scene.Title, "updated_at": scene.UpdatedAt, "asset_ids": s.sceneBoundAssets(scene), "references": references, "storyboards": boards, "missing_asset_ids": missing, "missing_character_ids": missingCharacters, "references_ready": ready, "storyboard_ready": boardReady, "active_storyboard_id": activeBoard, "scene_input_revision": inputRevision, "context_revision": current}, nil
}
func (s *Server) handleSceneReferences(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	scene, e := s.database.GetScene(chi.URLParam(r, "id"))
	if err != nil || e != nil || scene.ProjectID == nil || *scene.ProjectID != project {
		http.Error(w, "scene not found", 404)
		return
	}
	state, e := s.sceneReferenceState(r.Context(), scene)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	writeJSON(w, 200, state)
}
func (s *Server) handleSetSceneReference(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	scene, e := s.database.GetScene(chi.URLParam(r, "id"))
	if err != nil || e != nil || scene.ProjectID == nil || *scene.ProjectID != project {
		http.Error(w, "scene not found", 404)
		return
	}
	assetID := chi.URLParam(r, "asset_id")
	attach := r.Method == http.MethodPost
	asset, e := s.database.GetAsset(assetID)
	if attach {
		if e != nil || asset.ProjectID == nil || *asset.ProjectID != project {
			http.Error(w, "asset not found", 404)
			return
		}
		if asset.Type != "prop" && asset.Type != "location" {
			http.Error(w, "choose a prop or location; character references follow the scene cast", 400)
			return
		}
	} else {
		bound := false
		for _, id := range s.sceneBoundAssets(scene) {
			if id == assetID {
				bound = true
			}
		}
		if !bound {
			http.Error(w, "reference is not selected in this scene", 404)
			return
		}
	}
	var body struct {
		Expected time.Time `json:"expected_updated_at"`
		Replace  string    `json:"replace_asset_id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || body.Expected.IsZero() {
		http.Error(w, "expected_updated_at is required", 400)
		return
	}
	if attach {
		var metadata map[string]any
		_ = json.Unmarshal(asset.MetadataJSON, &metadata) // unreadable metadata has no review verdict
		if metadata["consistency_review"] == "rejected" {
			http.Error(w, "rejected assets cannot be used as scene references", 409)
			return
		}
		if _, e = s.imageURL(asset); e != nil {
			http.Error(w, "asset image is unavailable", 409)
			return
		}
	}
	if body.Replace != "" {
		if !attach || body.Replace == assetID {
			http.Error(w, "invalid version replacement", 400)
			return
		}
		previous, e := s.database.GetAsset(body.Replace)
		if e != nil || previous.ProjectID == nil || *previous.ProjectID != project || previous.Type != asset.Type || assetVariantGroup(previous) != assetVariantGroup(asset) {
			http.Error(w, "replacement must be a version of the same project asset", 409)
			return
		}
	}
	if e = s.database.SetSceneReference(project, scene.ID, assetID, attach, body.Replace, body.Expected); e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	scene, _ = s.database.GetScene(scene.ID)
	state, e := s.sceneReferenceState(r.Context(), scene)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	s.broker.Publish(map[string]any{"type": "scene_references_changed", "scene_id": scene.ID})
	writeJSON(w, 200, state)
}
func (s *Server) handleAssetUsage(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	a, e := s.database.GetAsset(chi.URLParam(r, "id"))
	if err != nil || e != nil || a.ProjectID == nil || *a.ProjectID != project {
		http.Error(w, "asset not found", 404)
		return
	}
	uses, e := s.database.AssetUses(project, a.ID)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	if value, ok := s.referenceLocks.Load("asset-version:" + a.ID); ok && len(value.(chan struct{})) > 0 {
		uses = append(uses, db.AssetUse{Kind: "generation", ID: a.ID, Name: "Asset version generation"})
	}
	stale := false
	if a.Type == "storyboard" {
		var metadata struct {
			Scene  string `json:"scene_id"`
			Inputs string `json:"scene_input_revision"`
		}
		_ = json.Unmarshal(a.MetadataJSON, &metadata) // unreadable metadata belongs to no scene
		if metadata.Scene != "" {
			if revision, e := s.currentSceneInputRevision(metadata.Scene); e == nil {
				stale = metadata.Inputs != revision
			}
		}
	}
	writeJSON(w, 200, map[string]any{"asset_id": a.ID, "uses": uses, "can_delete": len(uses) == 0, "review_stale": stale})
}

// sceneBoundAssets also recognizes legacy scene attachments; old projects keep their saved props.
func (s *Server) sceneBoundAssets(scene *models.Scene) []string {
	ids := append([]string{}, sceneStoryAssetIDs(scene)...)
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	attached := models.IDList(scene.AssetIDsJSON)
	for _, id := range attached {
		if seen[id] {
			continue
		}
		asset, err := s.database.GetAsset(id)
		if err == nil && asset.ProjectID != nil && scene.ProjectID != nil && *asset.ProjectID == *scene.ProjectID && (asset.Type == "prop" || asset.Type == "location") {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids
}
