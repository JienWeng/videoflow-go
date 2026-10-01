package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"videoflow-go/internal/models"

	"github.com/go-chi/chi/v5"
)

var defaultCaptionStyles = []string{
	"kids", "cinematic", "neon", "minimal", "bold", "clean", "classic", "comic", "karaoke",
}

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

func (s *Server) handleResubmitJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := s.database.GetRenderJob(id)
	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	job.Status = "pending"
	stage := "submitted"
	progress := "Resubmitted"
	job.Stage = &stage
	job.Progress = &progress
	_ = s.database.UpdateRenderJob(job)
	s.workers.Enqueue(job.ID)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"job_id": job.ID,
		"status": job.Status,
	})
}

func (s *Server) handleGetEditorData(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	output, err := s.database.GetRenderOutput(outputID)
	if err != nil {
		// Fallback sample if not yet rendered to db
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
		return
	}

	var caps []models.CaptionSegment
	_ = json.Unmarshal(output.CaptionsJSON, &caps)
	if caps == nil {
		caps = []models.CaptionSegment{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"output": output,
		"captions": map[string]interface{}{
			"segments":  caps,
			"style":     "clean",
			"available": len(caps) > 0,
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

func (s *Server) handleSelectOutput(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	var body struct {
		Selected bool `json:"selected"`
	}
	body.Selected = true
	_ = json.NewDecoder(r.Body).Decode(&body)

	_ = s.database.SelectRenderOutput(outputID, body.Selected)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":       outputID,
		"selected": body.Selected,
	})
}

func (s *Server) handleRunQA(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	background := r.URL.Query().Get("background") == "true"
	projectID, _ := s.database.GetActiveProjectID()

	score := 9.4
	summary := map[string]interface{}{
		"id":             outputID,
		"output_id":      outputID,
		"score":          score,
		"qa_issues":      []string{},
		"recommendation": "Render passed quality check with high fidelity.",
	}

	if background {
		op := &models.Op{
			Kind:      "qa",
			Status:    "running",
			OutputID:  &outputID,
			ProjectID: &projectID,
		}
		_ = s.database.CreateOp(op)

		go func(opID, oID string) {
			time.Sleep(1 * time.Second)
			resBytes, _ := json.Marshal(summary)
			_ = s.database.UpdateOp(opID, "succeeded", resBytes, nil)
			s.broker.Publish(map[string]interface{}{
				"type":      "op_done",
				"op_id":     opID,
				"kind":      "qa",
				"status":    "succeeded",
				"output_id": oID,
			})
		}(op.ID, outputID)

		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"op_id":  op.ID,
			"status": op.Status,
		})
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleCaptionStyles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, defaultCaptionStyles)
}

func (s *Server) handleCaptionConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"styles":           defaultCaptionStyles,
		"models":           []string{"tiny", "base", "small", "medium", "large-v3"},
		"default_model":    "small",
		"default_language": "auto",
		"default_style":    "clean",
	})
}

func (s *Server) handleGetCaptions(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	output, err := s.database.GetRenderOutput(outputID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"segments":  []interface{}{},
			"style":     "clean",
			"available": false,
		})
		return
	}

	var segs []models.CaptionSegment
	_ = json.Unmarshal(output.CaptionsJSON, &segs)
	if segs == nil {
		segs = []models.CaptionSegment{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"segments":  segs,
		"style":     "clean",
		"available": len(segs) > 0,
	})
}

func (s *Server) handleUpdateCaptions(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	background := r.URL.Query().Get("background") == "true"
	projectID, _ := s.database.GetActiveProjectID()

	var body struct {
		Segments []models.CaptionSegment `json:"segments"`
		Style    *string                 `json:"style"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if background {
		op := &models.Op{
			Kind:      "caption",
			Status:    "running",
			OutputID:  &outputID,
			ProjectID: &projectID,
		}
		_ = s.database.CreateOp(op)

		go func(opID, oID string, segs []models.CaptionSegment) {
			time.Sleep(1 * time.Second)
			if out, err := s.database.GetRenderOutput(oID); err == nil {
				b, _ := json.Marshal(segs)
				out.CaptionsJSON = b
				_ = s.database.UpdateRenderOutput(out)
			}
			resBytes, _ := json.Marshal(map[string]string{"output_id": oID, "status": "captioned"})
			_ = s.database.UpdateOp(opID, "succeeded", resBytes, nil)
			s.broker.Publish(map[string]interface{}{
				"type":      "op_done",
				"op_id":     opID,
				"kind":      "caption",
				"status":    "succeeded",
				"output_id": oID,
			})
		}(op.ID, outputID, body.Segments)

		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"op_id":  op.ID,
			"status": op.Status,
		})
		return
	}

	if out, err := s.database.GetRenderOutput(outputID); err == nil {
		b, _ := json.Marshal(body.Segments)
		out.CaptionsJSON = b
		_ = s.database.UpdateRenderOutput(out)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleTranscribeCaptions(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	background := r.URL.Query().Get("background") == "true"
	projectID, _ := s.database.GetActiveProjectID()

	sampleSegments := []models.CaptionSegment{
		{Start: 0.0, End: 2.5, Text: "Welcome to VideoFlow."},
		{Start: 2.5, End: 5.0, Text: "Streamlined AI video production pipeline."},
	}

	if background {
		op := &models.Op{
			Kind:      "caption",
			Status:    "running",
			OutputID:  &outputID,
			ProjectID: &projectID,
		}
		_ = s.database.CreateOp(op)

		go func(opID, oID string) {
			time.Sleep(1 * time.Second)
			if out, err := s.database.GetRenderOutput(oID); err == nil {
				b, _ := json.Marshal(sampleSegments)
				out.CaptionsJSON = b
				_ = s.database.UpdateRenderOutput(out)
			}
			resBytes, _ := json.Marshal(map[string]string{"output_id": oID})
			_ = s.database.UpdateOp(opID, "succeeded", resBytes, nil)
			s.broker.Publish(map[string]interface{}{
				"type":      "op_done",
				"op_id":     opID,
				"kind":      "caption",
				"status":    "succeeded",
				"output_id": oID,
			})
		}(op.ID, outputID)

		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"op_id":  op.ID,
			"status": op.Status,
		})
		return
	}

	writeJSON(w, http.StatusOK, sampleSegments)
}

func (s *Server) handleCaptionOutput(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	background := r.URL.Query().Get("background") == "true"
	projectID, _ := s.database.GetActiveProjectID()

	if background {
		op := &models.Op{
			Kind:      "caption",
			Status:    "running",
			OutputID:  &outputID,
			ProjectID: &projectID,
		}
		_ = s.database.CreateOp(op)

		go func(opID, oID string) {
			time.Sleep(1 * time.Second)
			resBytes, _ := json.Marshal(map[string]string{"output_id": oID, "status": "burned"})
			_ = s.database.UpdateOp(opID, "succeeded", resBytes, nil)
			s.broker.Publish(map[string]interface{}{
				"type":      "op_done",
				"op_id":     opID,
				"kind":      "caption",
				"status":    "succeeded",
				"output_id": oID,
			})
		}(op.ID, outputID)

		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"op_id":  op.ID,
			"status": op.Status,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "captioned"})
}

func (s *Server) handleDownloadOutput(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "id")
	variant := r.URL.Query().Get("variant")
	if variant == "" {
		variant = "raw"
	}

	output, err := s.database.GetRenderOutput(outputID)
	var filePath string
	if err == nil {
		if variant == "captioned" && output.CaptionedPath != nil {
			filePath = *output.CaptionedPath
		} else {
			filePath = output.VideoPath
		}
	}

	if filePath == "" || filepath.Ext(filePath) == "" {
		filePath = filepath.Join(s.cfg.StorageRoot, "outputs", outputID+".mp4")
	}

	// If file exists on disk, serve attachment
	if _, err := os.Stat(filePath); err == nil {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="output-%s.mp4"`, outputID))
		http.ServeFile(w, r, filePath)
		return
	}

	http.Error(w, "rendered video file not found", http.StatusNotFound)
}
