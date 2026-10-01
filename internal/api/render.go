package api

import (
	"encoding/json"
	"net/http"

	"videoflow-go/internal/models"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleStartRender(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}

	var body struct {
		SceneID  *string `json:"scene_id"`
		ShotID   *string `json:"shot_id"`
		Provider string  `json:"provider"`
		Model    string  `json:"model"`
		Prompt   string  `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if body.Provider == "" {
		body.Provider = s.cfg.DefaultVideoProvider
	}
	if body.Model == "" {
		body.Model = s.cfg.OpenRouterVideoModel
	}

	stage := "submitted"
	progress := "Queued for rendering"
	reqBytes, _ := json.Marshal(body)

	job := &models.RenderJob{
		ProjectID:   &projectID,
		SceneID:     body.SceneID,
		ShotID:      body.ShotID,
		Provider:    body.Provider,
		Model:       body.Model,
		Status:      "pending",
		Stage:       &stage,
		Progress:    &progress,
		RequestJSON: reqBytes,
	}

	if err := s.database.CreateRenderJob(job); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Enqueue to background worker pool
	s.workers.Enqueue(job.ID)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"job_id": job.ID,
		"status": job.Status,
	})
}

func (s *Server) handleRenderFromShot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SceneID string `json:"scene_id"`
		ShotID  string `json:"shot_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	shot, err := s.database.GetShot(body.ShotID)
	if err != nil {
		http.Error(w, "shot not found", http.StatusNotFound)
		return
	}

	projectID, _ := s.database.GetActiveProjectID()
	stage := "submitted"
	progress := "Queued from shot"
	reqBytes, _ := json.Marshal(shot)

	job := &models.RenderJob{
		ProjectID:   &projectID,
		SceneID:     &body.SceneID,
		ShotID:      &body.ShotID,
		Provider:    s.cfg.DefaultVideoProvider,
		Model:       s.cfg.OpenRouterVideoModel,
		Status:      "pending",
		Stage:       &stage,
		Progress:    &progress,
		RequestJSON: reqBytes,
	}

	if err := s.database.CreateRenderJob(job); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.workers.Enqueue(job.ID)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"job_id": job.ID,
		"status": job.Status,
	})
}

func (s *Server) handleListRenderJobs(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	jobs, err := s.database.ListRenderJobs(projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if jobs == nil {
		jobs = []models.RenderJob{}
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (s *Server) handleGetRenderJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := s.database.GetRenderJob(id)
	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	outputs, _ := s.database.ListOutputsForJob(id)
	if outputs == nil {
		outputs = []models.RenderOutput{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"job":     job,
		"outputs": outputs,
	})
}

func (s *Server) handleGetEditorData(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	// Return editor data block matching frontend schema
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"output": map[string]interface{}{
			"id":             outputID,
			"video_path":     "storage/outputs/sample.mp4",
			"captioned_path": nil,
			"score":          9.2,
			"qa_issues":      []string{},
		},
		"captions": map[string]interface{}{
			"segments":  []interface{}{},
			"style":     "clean",
			"available": false,
		},
		"shots":          []interface{}{},
		"scene":          nil,
		"total_duration": 5.0,
	})
}

func (s *Server) handleRetryOutput(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":    "pending",
		"output_id": outputID,
		"message":   "Retrying render",
	})
}

func (s *Server) handleCaptionConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"styles":           []string{"kids", "clean", "minimal"},
		"models":           []string{"tiny", "base", "small", "medium", "large-v3"},
		"default_model":    "small",
		"default_language": "auto",
		"default_style":    "clean",
	})
}
