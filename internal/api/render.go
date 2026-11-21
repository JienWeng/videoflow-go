package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"videoflow-go/internal/models"

	"github.com/go-chi/chi/v5"
)

var defaultCaptionStyles = []string{
	"kids", "cinematic", "neon", "minimal", "bold", "clean", "classic", "comic",
}

func (s *Server) handleStartRender(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}

	var body map[string]any
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		http.Error(w, "invalid request body", 400)
		return
	}
	delete(body, "director_context")
	delete(body, "director_keyframe")
	delete(body, "voice_policy")
	for _, field := range []string{"render_unit", "render_plan_id", "request_fingerprint"} {
		delete(body, field)
	}
	newVersion, _ := body["new_version"].(bool)
	delete(body, "new_version")

	provider, _ := body["provider"].(string)
	if provider == "" {
		provider, _ = s.routes().MediaSelection("video")
	}
	model, _ := body["model"].(string)
	if model == "" {
		selectedProvider, selectedModel := s.routes().MediaSelection("video")
		if selectedProvider == provider {
			model = selectedModel
		} else {
			model = s.routes().DefaultMediaModel(provider, "video")
		}
	}
	body["model"] = model
	var sceneID, shotID *string
	if value, ok := body["scene_id"].(string); ok {
		sceneID = &value
	}
	if value, ok := body["shot_id"].(string); ok {
		shotID = &value
	}
	if shotID != nil {
		shot, e := s.database.GetShot(*shotID)
		if e != nil || (sceneID != nil && *sceneID != shot.SceneID) {
			http.Error(w, "shot is not in this scene", 400)
			return
		}
		sceneID = &shot.SceneID
	}
	if sceneID != nil {
		scene, e := s.database.GetScene(*sceneID)
		if e != nil || scene.ProjectID == nil || *scene.ProjectID != projectID {
			http.Error(w, "scene is not in the active project", 400)
			return
		}
		unlock, e := s.lockSceneRender(*sceneID)
		if e != nil {
			http.Error(w, e.Error(), 409)
			return
		}
		defer unlock()
	}
	delete(body, "provider") // Provider is a local routing field; OpenRouter routing options use provider_options.
	if value, ok := body["provider_options"]; ok {
		body["provider"] = value
		delete(body, "provider_options")
	}

	stage := "submitted"
	progress := "Queued for rendering"
	reqBytes, _ := json.Marshal(body)

	job := &models.RenderJob{
		ProjectID:   &projectID,
		SceneID:     sceneID,
		ShotID:      shotID,
		Provider:    provider,
		Model:       model,
		Status:      "pending",
		Stage:       &stage,
		Progress:    &progress,
		RequestJSON: reqBytes,
	}

	if err := s.prepareDirectorVideoJob(r.Context(), job); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if sceneID != nil {
		if existing, e := s.existingSceneRender(job, newVersion); e != nil {
			http.Error(w, e.Error(), 409)
			return
		} else if existing != nil {
			writeJSON(w, 202, map[string]any{"job_id": existing.ID, "status": existing.Status, "reused": true})
			return
		}
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

func (s *Server) handleRenderFromShot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SceneID    string `json:"scene_id"`
		ShotID     string `json:"shot_id"`
		NewVersion bool   `json:"new_version"`
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

	projectID, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	scene, err := s.database.GetScene(shot.SceneID)
	if err != nil || scene.ProjectID == nil || *scene.ProjectID != projectID || (body.SceneID != "" && body.SceneID != shot.SceneID) {
		http.Error(w, "shot does not belong to this scene and active project", 400)
		return
	}
	body.SceneID = shot.SceneID
	unlock, e := s.lockSceneRender(body.SceneID)
	if e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	defer unlock()

	stage := "submitted"
	progress := "Queued from shot"
	reqBytes, _ := json.Marshal(map[string]any{"prompt": shot.Prompt})

	job := &models.RenderJob{
		ProjectID:   &projectID,
		SceneID:     &body.SceneID,
		ShotID:      &body.ShotID,
		Provider:    "",
		Model:       "",
		Status:      "pending",
		Stage:       &stage,
		Progress:    &progress,
		RequestJSON: reqBytes,
	}

	if err := s.prepareDirectorVideoJob(r.Context(), job); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if existing, e := s.existingSceneRender(job, body.NewVersion); e != nil {
		http.Error(w, e.Error(), 409)
		return
	} else if existing != nil {
		writeJSON(w, 202, map[string]any{"job_id": existing.ID, "status": existing.Status, "reused": true})
		return
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
	projectID, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	jobs, err := s.database.ShownRenderJobs(projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	views := make([]models.RenderJob, len(jobs))
	for i, job := range jobs {
		views[i] = renderJobView(job)
	}
	writeJSON(w, http.StatusOK, views)
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
		"job":     renderJobView(*job),
		"outputs": outputs,
	})
}

func (s *Server) handleResubmitJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.database.GetRenderJob(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "job not found", 404)
		return
	}
	if job.Status != "failed" && job.Status != "succeeded" {
		http.Error(w, "Wait for the active job, or reconcile an uncertain provider submission before retrying", 409)
		return
	}
	// Explicit retry starts a new generation; only restart reconciliation reuses the upstream ID.
	stage, progress := "submitted", "Retry queued"
	fresh := &models.RenderJob{ProjectID: job.ProjectID, SceneID: job.SceneID, ShotID: job.ShotID, Provider: job.Provider, ProviderEndpoint: job.ProviderEndpoint, Model: job.Model, Status: "pending", Stage: &stage, Progress: &progress, RequestJSON: job.RequestJSON}
	if fresh.Provider == "" {
		fresh.Provider = "openrouter"
	}
	var frozen map[string]json.RawMessage
	_ = json.Unmarshal(fresh.RequestJSON, &frozen)
	if frozen["director_context"] != nil {
		err = s.validateFrozenVideo(r.Context(), fresh)
		if err == nil {
			err = s.prepareVideoJob(fresh)
		}
	} else {
		err = s.prepareDirectorVideoJob(r.Context(), fresh)
	}
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	created, err := s.database.CreateRenderRetry(job.ID, fresh)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if created {
		s.workers.Enqueue(fresh.ID)
	}
	writeJSON(w, 202, map[string]any{"job_id": fresh.ID, "status": fresh.Status})
}

func (s *Server) handleRetryOutput(w http.ResponseWriter, r *http.Request) {
	output, err := s.database.GetRenderOutput(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "output not found", 404)
		return
	}
	route := chi.RouteContext(r.Context())
	route.URLParams.Add("id", output.RenderJobID)
	s.handleResubmitJob(w, r)
}

func (s *Server) handleCaptionConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"styles":           defaultCaptionStyles,
		"default_language": "auto",
		"default_style":    "clean",
	})
}

func (s *Server) handleCaptionOutput(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	output, err := s.database.GetRenderOutput(outputID)
	if err != nil {
		http.Error(w, "output not found", 404)
		return
	}
	var body struct {
		Style string `json:"style"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid caption request", 400)
			return
		}
	}
	var captions struct {
		Segments []models.CaptionSegment `json:"segments"`
		Style    string                  `json:"style"`
	}
	if json.Unmarshal(output.CaptionsJSON, &captions) != nil {
		json.Unmarshal(output.CaptionsJSON, &captions.Segments)
	}
	if len(captions.Segments) == 0 {
		http.Error(w, "This video has no saved captions. Add captions in Edit video and export from there.", 400)
		return
	}
	if body.Style != "" {
		captions.Style = body.Style
	}
	if captions.Style == "" {
		captions.Style = "clean"
	}
	s.operation(w, r, &models.Op{Kind: "caption", OutputID: &outputID}, func(ctx context.Context) (any, error) {
		return s.saveBurnedCaptions(ctx, output, captions.Segments, captions.Style)
	})
}

func (s *Server) handleDownloadOutput(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	output, err := s.database.GetRenderOutput(outputID)
	if err != nil {
		http.Error(w, "output not found", 404)
		return
	}
	path := output.VideoPath
	if r.URL.Query().Get("variant") == "captioned" {
		if output.CaptionedPath == nil {
			http.Error(w, "captioned video has not been generated", 404)
			return
		}
		path = *output.CaptionedPath
	}
	full, err := s.storagePath(path)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if _, err = os.Stat(full); err != nil {
		http.Error(w, "video file not found", 404)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="output-%s.mp4"`, outputID))
	http.ServeFile(w, r, full)
}
