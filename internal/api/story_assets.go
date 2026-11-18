package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"videoflow-go/internal/models"
)

func matchingStoryAsset(item plannedAsset, catalog []models.Asset) string {
	normalize := func(v string) string { return strings.ToLower(strings.Join(strings.Fields(v), " ")) }
	for _, a := range catalog {
		if a.Type == item.Type && normalize(a.Name) == normalize(item.Name) && normalize(a.Description) == normalize(item.Description) {
			return a.ID
		}
	}
	return ""
}

func (s *Server) storyAssetCatalog(project *string) ([]models.Asset, error) {
	if project == nil {
		return nil, fmt.Errorf("asset project is required")
	}
	assets, err := s.database.ListAssets(*project)
	if err != nil {
		return nil, err
	}
	result := []models.Asset{}
	for _, a := range assets {
		if a.Type != "prop" && a.Type != "location" {
			continue
		}
		var metadata map[string]any
		_ = json.Unmarshal(a.MetadataJSON, &metadata)
		if metadata["consistency_review"] == "rejected" {
			continue
		}
		if _, err := s.imageURL(&a); err != nil {
			continue
		}
		result = append(result, a)
	}
	return result, nil
}

// Materialization always rechecks the current project library under a project lock.
func (s *Server) materializeStoryAssets(ctx context.Context, scene *models.Scene, plan *assetPlan) ([]models.Asset, error) {
	ctx = sceneBrief(scene).context(ctx)
	if scene.ProjectID == nil {
		return nil, fmt.Errorf("scene project is required")
	}
	value, _ := s.referenceLocks.LoadOrStore("assets:"+*scene.ProjectID, make(chan struct{}, 1))
	lock := value.(chan struct{})
	select {
	case lock <- struct{}{}:
		defer func() { <-lock }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	result := []models.Asset{}
	for index, item := range plan.Assets {
		if (item.Type != "prop" && item.Type != "location") || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Description) == "" {
			return result, fmt.Errorf("invalid asset plan item")
		}
		catalog, err := s.storyAssetCatalog(scene.ProjectID)
		if err != nil {
			return result, err
		}
		id := item.ExistingAssetID
		if id == "" {
			id = matchingStoryAsset(item, catalog)
		}
		plan.Assets[index].Reuse = id != ""
		if id != "" && item.ExistingAssetID == "" {
			plan.Assets[index].Reason = "An existing library asset has the same name, type and appearance description."
		}
		var asset *models.Asset
		if id != "" {
			for i := range catalog {
				if catalog[i].ID == id && catalog[i].Type == item.Type {
					asset = &catalog[i]
					break
				}
			}
			if asset == nil {
				return result, fmt.Errorf("selected library asset is no longer available")
			}
		}
		if asset == nil {
			generated, err := s.generateAssets(guidedStoryContext(ctx, *scene.ProjectID, nil), scene.ProjectID, nil, "", item.Name, item.Type, productionAssetPrompt(item.Type, item.Description), "1:1", nil)
			if err != nil {
				return result, err
			}
			if len(generated) == 0 {
				return result, fmt.Errorf("no asset image returned")
			}
			asset = &generated[0]
			// Prompt scaffolding must not change identity matching or library descriptions.
			asset.Description = item.Description
			if err := s.database.UpdateAsset(asset); err != nil {
				return result, err
			}
		}
		plan.Assets[index].AssetID = asset.ID
		// Reload after each image call; preserve dialogue, generation brief and prior links.
		current, err := s.database.GetScene(scene.ID)
		if err != nil {
			return result, err
		}
		var metadata map[string]json.RawMessage
		if json.Unmarshal(current.SceneJSON, &metadata) != nil || metadata == nil {
			metadata = map[string]json.RawMessage{}
		}
		var refs, all []string
		_ = json.Unmarshal(metadata["story_asset_ids"], &refs)
		_ = json.Unmarshal(current.AssetIDsJSON, &all)
		appendUnique := func(ids []string, id string) []string {
			for _, existing := range ids {
				if existing == id {
					return ids
				}
			}
			return append(ids, id)
		}
		refs = appendUnique(refs, asset.ID)
		all = appendUnique(all, asset.ID)
		metadata["story_asset_ids"], _ = json.Marshal(refs)
		current.SceneJSON, _ = json.Marshal(metadata)
		current.AssetIDsJSON, _ = json.Marshal(all)
		if err = s.database.UpdateScene(current); err != nil {
			return result, err
		}
		*scene = *current
		result = append(result, *asset)
	}
	return result, nil
}

func sceneStoryAssetIDs(scene *models.Scene) []string {
	var metadata struct {
		IDs []string `json:"story_asset_ids"`
	}
	_ = json.Unmarshal(scene.SceneJSON, &metadata)
	return metadata.IDs
}

// Keep reusable design assets free of scene-specific action and character contamination.
func productionAssetPrompt(kind, description string) string {
	switch kind {
	case "location":
		return description + "\nReusable location reference: show a single readable space with foreground, middle ground, background, entrances and pathways. Preserve stable architecture, lighting sources and spatial landmarks. Do not include characters, story action, captions or watermarks. Respect the selected visual medium."
	case "prop":
		return description + "\nReusable prop reference: clearly show the complete object, silhouette, material, colors and distinguishing structural details against a simple background. No hands, characters, story action, captions or watermarks. Respect the selected visual medium."
	}
	return description
}
