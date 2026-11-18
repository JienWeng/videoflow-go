package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"videoflow-go/internal/models"
)

func defaultStyleGuide(projectID string) models.StyleGuide {
	return models.StyleGuide{
		ProjectID:   &projectID,
		Name:        "Natural",
		StylePrompt: "Use the requested visual medium. When unspecified, use natural proportions, material textures and practical lighting.",
	}
}

func styleDirection(style *models.StyleGuide) string {
	parts := []string{strings.TrimSpace(style.StylePrompt)}
	if palette := strings.TrimSpace(style.Palette); palette != "" {
		parts = append(parts, "Palette: "+palette)
	}
	if lighting := strings.TrimSpace(style.Lighting); lighting != "" {
		parts = append(parts, "Lighting: "+lighting)
	}
	return strings.Join(parts, "\n")
}

func (s *Server) sceneVisualStyle(scene *models.Scene) string {
	if style := strings.TrimSpace(sceneBrief(scene).Style); style != "" {
		return style
	}
	return s.stylePrompt(scene.ProjectID)
}

func (s *Server) handleGetStyle(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	style, err := s.database.GetStyleGuide(projectID)
	if err != nil {
		fallback := defaultStyleGuide(projectID)
		style = &fallback
	}
	writeJSON(w, http.StatusOK, style)
}

func (s *Server) handleUpdateStyle(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	current, loadErr := s.database.GetStyleGuide(projectID)
	if loadErr != nil {
		fallback := defaultStyleGuide(projectID)
		current = &fallback
	}
	style := *current
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&style); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if style.ProjectID != nil && *style.ProjectID != projectID {
		http.Error(w, "project changed; reload before saving its style", 409)
		return
	}
	style.ProjectID = &projectID
	if err := s.database.UpsertStyleGuide(&style); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, style)
}

func (s *Server) handleIngestStyle(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var body struct {
		AssetIDs []string `json:"asset_ids"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.operation(w, r, &models.Op{Kind: "style_ingest", ProjectID: &projectID}, func(ctx context.Context) (any, error) {
		ids := body.AssetIDs
		if len(ids) == 0 {
			assets, err := s.database.ListAssets(projectID)
			if err != nil {
				return nil, err
			}
			for _, asset := range assets {
				if asset.Type == "character_reference" || asset.Type == "location" {
					ids = append(ids, asset.ID)
					if len(ids) == 4 {
						break
					}
				}
			}
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("upload at least one image reference before extracting style")
		}
		if len(ids) > 4 {
			ids = ids[:4]
		}
		images := []string{}
		for _, id := range ids {
			asset, err := s.database.GetAsset(id)
			if err != nil {
				return nil, err
			}
			if asset.ProjectID == nil || *asset.ProjectID != projectID {
				return nil, fmt.Errorf("reference is outside active project")
			}
			image, err := s.imageURL(asset)
			if err != nil {
				return nil, err
			}
			images = append(images, image)
		}
		var style models.StyleGuide
		if err = s.agentJSON(ctx, "style_agent", "Analyze the image references. Return name, style_prompt, palette, lighting, audience, tone. Describe only visible style features.", "Extract a consistent production style.", images, &style); err != nil {
			return nil, err
		}
		if style.StylePrompt == "" {
			return nil, fmt.Errorf("OpenRouter returned no style prompt")
		}
		style.ProjectID = &projectID
		if err = s.database.UpsertStyleGuide(&style); err != nil {
			return nil, err
		}
		return style, nil
	})
}
