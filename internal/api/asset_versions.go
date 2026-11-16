package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"time"
	"videoflow-go/internal/models"
)

func assetVariantGroup(asset *models.Asset) string {
	var metadata struct {
		Group string `json:"variant_group_id"`
	}
	_ = json.Unmarshal(asset.MetadataJSON, &metadata)
	if metadata.Group != "" {
		return metadata.Group
	}
	return asset.ID
}

func (s *Server) handleRegenerateAsset(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	source, e := s.database.GetAsset(chi.URLParam(r, "id"))
	if err != nil || e != nil || source.ProjectID == nil || *source.ProjectID != project {
		http.Error(w, "asset not found", 404)
		return
	}
	if source.Type != "prop" && source.Type != "location" {
		http.Error(w, "regenerate character references from the character, and storyboards from their scene studio", 400)
		return
	}
	var body struct {
		Expected time.Time `json:"expected_updated_at"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || !body.Expected.Equal(source.UpdatedAt) {
		http.Error(w, "asset changed; reopen it before generating a new version", 409)
		return
	}
	if source.Description == "" {
		http.Error(w, "add an asset description before generating a new version", 400)
		return
	}
	s.operation(w, r, &models.Op{Kind: "asset_regeneration", ProjectID: &project}, func(ctx context.Context) (any, error) {
		value, _ := s.referenceLocks.LoadOrStore("asset-version:"+source.ID, make(chan struct{}, 1))
		lock := value.(chan struct{})
		select {
		case lock <- struct{}{}:
			defer func() { <-lock }()
		default:
			return nil, fmt.Errorf("a version of this asset is already being generated")
		}
		current, e := s.database.GetAsset(source.ID)
		if e != nil || !current.UpdatedAt.Equal(source.UpdatedAt) {
			return nil, fmt.Errorf("asset changed before generation")
		}
		var identity map[string]any
		_ = json.Unmarshal(source.MetadataJSON, &identity)
		ctx = guidedStoryContext(ctx, project, nil)
		references := []string{}
		if models.ReferenceApproved(source.MetadataJSON) {
			if reference, e := s.imageURL(source); e == nil {
				if !s.cfg.UseLocalText {
					references = append(references, reference)
				}
				binding, _ := ctx.Value(directorBindingKey{}).(directorBinding)
				binding.Selection.AssetIDs = []string{source.ID}
				ctx = context.WithValue(ctx, directorBindingKey{}, binding)
			}
		}
		locks, _ := json.Marshal(identity["locked_traits"])
		prompt := source.Description + "\nGenerate a new version of the same " + source.Type + ". Preserve its identity and these locked traits: " + string(locks)
		assets, e := s.generateAssets(ctx, &project, nil, "", source.Name, source.Type, prompt, "1:1", references)
		if e != nil {
			return nil, e
		}
		ids := []string{}
		for i := range assets {
			a := &assets[i]
			var metadata map[string]any
			_ = json.Unmarshal(a.MetadataJSON, &metadata)
			metadata["source_asset_id"] = source.ID
			metadata["source_updated_at"] = source.UpdatedAt
			metadata["variant_group_id"] = assetVariantGroup(source)
			metadata["consistency_review"] = "pending"
			if identity["locked_traits"] != nil {
				metadata["locked_traits"] = identity["locked_traits"]
			}
			a.MetadataJSON, _ = json.Marshal(metadata)
			a.Description = source.Description
			if e = s.database.UpdateAsset(a); e != nil {
				return nil, e
			}
			ids = append(ids, a.ID)
		}
		return map[string]any{"source_asset_id": source.ID, "asset_ids": ids, "assets": assets}, nil
	})
}
