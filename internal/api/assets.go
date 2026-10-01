package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"videoflow-go/internal/models"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleListAssets(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	assets, err := s.database.ListAssets(projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if assets == nil {
		assets = []models.Asset{}
	}
	writeJSON(w, http.StatusOK, assets)
}

func (s *Server) handleAssetTypes(w http.ResponseWriter, r *http.Request) {
	types := []string{
		"character_reference",
		"location",
		"prop",
		"video_reference",
		"audio_reference",
		"voice",
		"output_video",
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"types": types})
}

func (s *Server) handleGetAsset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	asset, err := s.database.GetAsset(id)
	if err != nil {
		http.Error(w, "asset not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, asset)
}

func (s *Server) handleDeleteAsset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.database.DeleteAsset(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleUploadAsset(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(50 << 20); err != nil { // 50 MB
		http.Error(w, "file too large", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file parameter", http.StatusBadRequest)
		return
	}
	defer file.Close()

	assetType := r.FormValue("type")
	if assetType == "" {
		assetType = "character_reference"
	}

	projectID, _ := s.database.GetActiveProjectID()
	destDir := filepath.Join(s.cfg.StorageRoot, "assets")
	_ = os.MkdirAll(destDir, 0755)

	assetID := "asset_" + uuid.New().String()[:8]
	ext := filepath.Ext(header.Filename)
	destPath := filepath.Join(destDir, assetID+ext)

	out, err := os.Create(destPath)
	if err != nil {
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		http.Error(w, "failed to write file", http.StatusInternalServerError)
		return
	}

	asset := &models.Asset{
		ID:           assetID,
		ProjectID:    &projectID,
		Type:         assetType,
		Name:         header.Filename,
		FilePath:     destPath,
		TagsJSON:     []byte("[]"),
		MetadataJSON: []byte("{}"),
	}
	if err := s.database.CreateAsset(asset); err != nil {
		http.Error(w, "failed to save asset to db", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, asset)
}

func (s *Server) handleRecogniseAsset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	asset, err := s.database.GetAsset(id)
	if err != nil {
		http.Error(w, "asset not found", http.StatusNotFound)
		return
	}

	var body struct {
		Description string `json:"description"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if body.Description != "" {
		asset.Description = body.Description
		_ = s.database.UpdateAsset(asset)
	}

	writeJSON(w, http.StatusOK, asset)
}
