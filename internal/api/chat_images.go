package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"videoflow-go/internal/models"
)

// Direct image creation uses the same tracked native-first image path as scene assets.
func (s *Server) handleGenerateImage(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	var body struct {
		Name   string `json:"name"`
		Prompt string `json:"prompt"`
		Type   string `json:"type"`
		Aspect string `json:"aspect_ratio"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
	d.DisallowUnknownFields()
	if d.Decode(&body) != nil || strings.TrimSpace(body.Prompt) == "" || len(body.Prompt) > 12000 || len(body.Name) > 120 {
		http.Error(w, "provide an image prompt (up to 12000 bytes) and a name (up to 120)", 400)
		return
	}
	if d.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid request", 400)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		body.Name = "Generated image"
	}
	if body.Type == "" {
		body.Type = "other"
	}
	if body.Aspect == "" {
		body.Aspect = "1:1"
	}
	switch body.Type {
	case "prop", "location", "background", "other":
	default:
		http.Error(w, "use the character or scene workflow for this image type", 400)
		return
	}
	switch body.Aspect {
	case "1:1", "9:16", "16:9", "3:4", "4:3", "21:9":
	default:
		http.Error(w, "unsupported aspect ratio", 400)
		return
	}
	s.operation(w, r, &models.Op{Kind: "image_generation", ProjectID: &project}, func(ctx context.Context) (any, error) {
		assets, err := s.generateAssets(ctx, &project, nil, "", body.Name, body.Type, body.Prompt, body.Aspect, nil)
		if err != nil {
			return nil, err
		}
		ids := []string{}
		for _, a := range assets {
			ids = append(ids, a.ID)
		}
		return map[string]any{"asset_ids": ids}, nil
	})
}
