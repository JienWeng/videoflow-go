package api

import (
	"encoding/json"
	"net/http"
	"os"

	"videoflow-go/internal/models"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.database.ListProjects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if projects == nil {
		projects = []models.Project{}
	}
	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) handleGetActiveProject(w http.ResponseWriter, r *http.Request) {
	proj, err := s.database.GetActiveProject()
	if err != nil {
		http.Error(w, "no active project", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, proj)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		body.Name = "Untitled Project"
	}
	proj, err := s.database.CreateProject(body.Name, body.Description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, proj)
}

func (s *Server) handleActivateProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.database.ActivateProject(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "active_project_id": id})
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	p, err := s.database.GetProject(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	if body.Name != nil {
		p.Name = *body.Name
	}
	if body.Description != nil {
		p.Description = *body.Description
	}
	if err := s.database.UpdateProject(id, p.Name, p.Description); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p, err = s.database.GetProject(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.database.DeleteProject(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleExportProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := s.database.GetProject(id)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	bundle := models.ProjectBundle{Project: *p}
	bundle.Characters, err = s.database.ListCharacters(id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	bundle.Scenes, err = s.database.ListScenes(id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	bundle.Assets, err = s.database.ListAssets(id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	bundle.Scripts, err = s.database.ListScripts(id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	bundle.Style, _ = s.database.GetStyleGuide(id)
	for _, scene := range bundle.Scenes {
		shots, err := s.database.ListShots(scene.ID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		bundle.Shots = append(bundle.Shots, shots...)
	}
	writeJSON(w, http.StatusOK, bundle)
}

func (s *Server) handleImportProject(w http.ResponseWriter, r *http.Request) {
	var bundle models.ProjectBundle
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
		http.Error(w, "Choose a VideoFlow project JSON export (ZIP files are not supported)", http.StatusBadRequest)
		return
	}
	// JSON references media already on this backend; never import paths outside
	// its storage root or pretend that missing media was included in the JSON.
	for _, asset := range bundle.Assets {
		path, err := s.storagePath(asset.FilePath)
		if err != nil {
			http.Error(w, "Imported media must reference files in this backend's storage folder", 400)
			return
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			http.Error(w, "Referenced media is missing; restore the data volume on this host before importing", 400)
			return
		}
	}

	project, err := s.database.ImportProject(bundle)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, project)
}
