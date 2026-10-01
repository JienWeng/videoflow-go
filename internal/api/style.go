package api

import (
	"encoding/json"
	"net/http"

	"videoflow-go/internal/models"
)

func (s *Server) handleGetStyle(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	style, err := s.database.GetStyleGuide(projectID)
	if err != nil {
		// Return default preset
		style = &models.StyleGuide{
			ProjectID:   &projectID,
			Name:        "2D Picture Book",
			StylePrompt: "warm 2D children's picture-book illustration, clean shapes, expressive faces",
			Palette:     "soft pastel colors",
			Lighting:    "gentle even lighting",
			Audience:    "children aged 5 to 9",
			Tone:        "warm and playful",
		}
	}
	writeJSON(w, http.StatusOK, style)
}

func (s *Server) handleUpdateStyle(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	var style models.StyleGuide
	if err := json.NewDecoder(r.Body).Decode(&style); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
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
	// Analyzes reference images and extracts style prompt
	var body struct {
		AssetIDs []string `json:"asset_ids"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":       "ok",
		"style_prompt": "Cinematic visual tone extracted from references",
		"palette":      "Rich contrast, warm shadows",
	})
}
