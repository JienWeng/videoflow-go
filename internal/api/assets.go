package api

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"videoflow-go/internal/models"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleListAssets(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.activeProjectID(r.Context())
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
		"storyboard",
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
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
	if err := r.ParseMultipartForm(50 << 20); err != nil { // 50 MB
		http.Error(w, "file too large", http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()
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

	projectID, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", http.StatusBadRequest)
		return
	}
	characterID := r.FormValue("character_id")
	if characterID != "" {
		c, err := s.database.GetCharacter(characterID)
		if err != nil || c.ProjectID == nil || *c.ProjectID != projectID {
			http.Error(w, "character not found in active project", http.StatusBadRequest)
			return
		}
	}

	destDir := filepath.Join(s.cfg.StorageRoot, "assets")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		http.Error(w, "failed to prepare upload storage", 500)
		return
	}

	assetID := "asset_" + uuid.New().String()[:8]
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if assetType == "character_reference" || ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp" {
		headerBytes := make([]byte, 512)
		n, _ := file.Read(headerBytes)
		if _, err = file.Seek(0, 0); err != nil {
			http.Error(w, "could not inspect uploaded image", 400)
			return
		}
		contentType := http.DetectContentType(headerBytes[:n])
		extensions := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp"}
		normal, ok := extensions[contentType]
		if !ok {
			http.Error(w, "reference must contain an actual PNG, JPEG, GIF or WebP image", 400)
			return
		}
		if contentType != "image/webp" {
			config, _, err := image.DecodeConfig(io.LimitReader(file, 50<<20))
			if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 16384 || config.Height > 16384 || int64(config.Width)*int64(config.Height) > 100000000 {
				http.Error(w, "reference image is invalid or exceeds supported dimensions", 400)
				return
			}
			if _, err = file.Seek(0, 0); err != nil {
				http.Error(w, "could not read uploaded image", 400)
				return
			}
		}
		ext = normal
	}
	destPath := filepath.Join(destDir, assetID+ext)

	out, err := os.Create(destPath)
	if err != nil {
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}
	saved := false
	defer func() {
		out.Close()
		if !saved {
			os.Remove(destPath)
		}
	}()

	if _, err := io.Copy(out, file); err != nil {
		http.Error(w, "failed to write file", http.StatusInternalServerError)
		return
	}

	asset := &models.Asset{
		ID:           assetID,
		ProjectID:    &projectID,
		Type:         assetType,
		Name:         header.Filename,
		FilePath:     filepath.ToSlash(filepath.Join("storage", "assets", assetID+ext)),
		TagsJSON:     []byte("[]"),
		MetadataJSON: []byte("{}"),
	}
	if characterID != "" {
		asset.CharacterID = &characterID
	}
	if err := s.database.CreateUploadedAsset(asset); err != nil {
		http.Error(w, "failed to save asset to db", http.StatusInternalServerError)
		return
	}

	saved = true
	writeJSON(w, http.StatusOK, asset)
}

func (s *Server) handleRecogniseAsset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	asset, err := s.database.GetAsset(id)
	if err != nil {
		http.Error(w, "asset not found", http.StatusNotFound)
		return
	}

	project, _ := s.activeProjectID(r.Context())
	if asset.ProjectID == nil || *asset.ProjectID != project {
		http.Error(w, "asset not found in active project", 404)
		return
	}
	var body struct {
		Description string `json:"description"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	image, err := s.imageURL(asset)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var result struct {
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	if err = s.agentJSON(r.Context(), "asset_recogniser", "Describe the visible subjects, appearance, objects and visual style. Return description and tags.", "User notes: "+body.Description, []string{image}, &result); err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	if result.Description == "" {
		http.Error(w, fmt.Sprint("OpenRouter returned no description"), 502)
		return
	}
	asset.Description = result.Description
	asset.TagsJSON, _ = json.Marshal(result.Tags)
	if asset.ProjectID == nil {
		http.Error(w, "asset project unavailable", 409)
		return
	}
	if err = s.database.UpdateAssetRecognition(*asset.ProjectID, asset.ID, asset.Description, asset.TagsJSON); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	asset, _ = s.database.GetAsset(asset.ID)
	writeJSON(w, http.StatusOK, asset)
}
