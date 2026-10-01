package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"videoflow-go/internal/config"
	"videoflow-go/internal/db"
	"videoflow-go/internal/events"
	"videoflow-go/internal/jobs"
	"videoflow-go/internal/models"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
)

type Server struct {
	cfg      *config.Config
	database *db.DB
	broker   *events.Broker
	workers  *jobs.WorkerPool
	router   *chi.Mux
}

func NewServer(cfg *config.Config, database *db.DB, broker *events.Broker, workers *jobs.WorkerPool) *Server {
	s := &Server{
		cfg:      cfg,
		database: database,
		broker:   broker,
		workers:  workers,
		router:   chi.NewRouter(),
	}
	s.setupRoutes()
	return s
}

func (s *Server) Router() http.Handler {
	return s.router
}

func (s *Server) setupRoutes() {
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Timeout(60 * time.Second))

	// CORS matching FastAPI frontend origins
	s.router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://localhost:4173", "http://127.0.0.1:4173"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Static storage server
	_ = os.MkdirAll(s.cfg.StorageRoot, 0755)
	fileServer := http.FileServer(http.Dir(s.cfg.StorageRoot))
	s.router.Handle("/storage/*", http.StripPrefix("/storage/", fileServer))

	// Health endpoint
	s.router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"status":       "ok",
			"missing_keys": s.cfg.MissingKeys(),
			"text_model":   s.cfg.MiniMaxTextModel,
			"image_model":  s.cfg.AtlasImageModel,
			"video_model":  s.cfg.AtlasVideoModel,
			"server":       "videoflow-go-v1",
		}
		writeJSON(w, http.StatusOK, resp)
	})

	// Server-Sent Events (SSE) endpoint
	s.router.Get("/events", s.handleEvents)

	// Projects
	s.router.Get("/projects", s.handleListProjects)
	s.router.Post("/projects", s.handleCreateProject)
	s.router.Post("/projects/{id}/activate", s.handleActivateProject)

	// Characters
	s.router.Get("/characters", s.handleListCharacters)

	// Scenes
	s.router.Get("/scenes", s.handleListScenes)

	// Assets
	s.router.Get("/assets", s.handleListAssets)
	s.router.Post("/assets/upload", s.handleUploadAsset)

	// Render
	s.router.Get("/render/{id}", s.handleGetRenderJob)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.broker.Subscribe()
	defer s.broker.Unsubscribe(ch)

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprint(w, msg)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

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

func (s *Server) handleListCharacters(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	chars, err := s.database.ListCharacters(projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if chars == nil {
		chars = []models.Character{}
	}
	writeJSON(w, http.StatusOK, chars)
}

func (s *Server) handleListScenes(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	scenes, err := s.database.ListScenes(projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if scenes == nil {
		scenes = []models.Scene{}
	}
	writeJSON(w, http.StatusOK, scenes)
}

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

func (s *Server) handleGetRenderJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := s.database.GetRenderJob(id)
	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
