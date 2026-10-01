package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"videoflow-go/internal/models"
)

func (s *Server) handleVideoPreflight(w http.ResponseWriter, r *http.Request) {
	aspectRatio := r.URL.Query().Get("aspect_ratio")
	if aspectRatio == "" {
		aspectRatio = "9:16"
	}
	style := r.URL.Query().Get("style")
	if style == "" {
		style = "2d-picture-book"
	}

	missing := []map[string]string{}
	if s.cfg.OpenRouterAPIKey == "" && s.cfg.AtlasCloudAPIKey == "" {
		missing = append(missing, map[string]string{
			"route":   "media",
			"setting": "/settings",
			"reason":  "Configure an OpenRouter or AtlasCloud API key in settings.",
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ready":   len(missing) == 0,
		"missing": missing,
		"effective_routes": map[string]string{
			"video_provider": s.cfg.DefaultVideoProvider,
			"video_model":    s.cfg.OpenRouterVideoModel,
			"image_model":    s.cfg.OpenRouterImageModel,
		},
		"style": map[string]string{
			"selected": style,
			"aspect":   aspectRatio,
		},
	})
}

func (s *Server) handleGenerateVideo(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}

	var body struct {
		Idea             string `json:"idea"`
		TargetDuration   int    `json:"target_duration"`
		SceneCount       int    `json:"scene_count"`
		Style            string `json:"style"`
		AspectRatio      string `json:"aspect_ratio"`
		Language         string `json:"language"`
		ConversationMode string `json:"conversation_mode"`
		Instruction      string `json:"instruction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Idea == "" {
		http.Error(w, "idea is required", http.StatusBadRequest)
		return
	}

	if body.SceneCount <= 0 {
		body.SceneCount = 3
	}
	if body.TargetDuration <= 0 {
		body.TargetDuration = body.SceneCount * 5
	}
	if body.AspectRatio == "" {
		body.AspectRatio = "9:16"
	}

	op := &models.Op{
		Kind:      "video_generation",
		Status:    "running",
		ProjectID: &projectID,
	}
	if err := s.database.CreateOp(op); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Run guided video pipeline asynchronously
	go func(opID, pid string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		stages := []string{}
		recordStage := func(stage string) {
			stages = append(stages, stage)
			s.broker.Publish(map[string]interface{}{
				"type":       "pipeline_stage",
				"op_id":      opID,
				"stage":      stage,
				"stages":     stages,
				"project_id": pid,
			})
		}

		// Stage 1: Story / Script
		draft, err := s.agents.GenerateScript(ctx, body.Idea, body.SceneCount, body.TargetDuration, body.AspectRatio)
		if err != nil {
			errStr := err.Error()
			_ = s.database.UpdateOp(opID, "failed", nil, &errStr)
			return
		}
		recordStage("story")

		draftBytes, _ := json.Marshal(draft)
		script := &models.Script{
			ProjectID: &pid,
			Idea:      body.Idea,
			Title:     draft.Title,
			Summary:   draft.Summary,
			DraftJSON: draftBytes,
		}
		_ = s.database.CreateScript(script)

		// Stage 2: Scenes & Shots
		createdScenes := []models.Scene{}
		for i, sc := range draft.Scenes {
			scene := models.Scene{
				ProjectID:   &pid,
				ScriptID:    &script.ID,
				SceneOrder:  i + 1,
				Title:       sc.Title,
				Summary:     sc.Summary,
				Duration:    sc.Duration,
				AspectRatio: sc.AspectRatio,
			}
			_ = s.database.CreateScene(&scene)
			createdScenes = append(createdScenes, scene)

			// Generate shots for scene
			shots, _ := s.agents.GenerateShots(ctx, scene.Title, scene.Summary, scene.Duration)
			for _, sh := range shots {
				shot := models.Shot{
					SceneID:   scene.ID,
					ShotOrder: sh.Order,
					Duration:  sh.Duration,
					Prompt:    sh.Prompt,
					Camera:    &sh.Camera,
					Movement:  &sh.Movement,
				}
				_ = s.database.CreateShot(&shot)
			}
		}
		recordStage("scenes")
		recordStage("shots")

		// Stage 3: Dialogue
		recordStage("dialogue")

		// Stage 4: Visuals
		recordStage("visuals")

		// Stage 5: Render Jobs
		renderJobIDs := []string{}
		for _, sc := range createdScenes {
			stage := "submitted"
			progress := "Queued for render"
			job := models.RenderJob{
				ProjectID: &pid,
				SceneID:   &sc.ID,
				Provider:  s.cfg.DefaultVideoProvider,
				Model:     s.cfg.OpenRouterVideoModel,
				Status:    "pending",
				Stage:     &stage,
				Progress:  &progress,
			}
			_ = s.database.CreateRenderJob(&job)
			renderJobIDs = append(renderJobIDs, job.ID)
			s.workers.Enqueue(job.ID)
		}
		recordStage("render")

		sceneIDs := make([]string, len(createdScenes))
		for i, sc := range createdScenes {
			sceneIDs[i] = sc.ID
		}

		result := map[string]interface{}{
			"project_id":     pid,
			"script_id":      script.ID,
			"scene_ids":      sceneIDs,
			"render_job_ids": renderJobIDs,
			"stages":         stages,
			"status":         "rendering",
		}
		resultBytes, _ := json.Marshal(result)
		_ = s.database.UpdateOp(opID, "succeeded", resultBytes, nil)

		log.Printf("Guided video pipeline completed for op %s (%d scenes, %d render jobs)", opID, len(sceneIDs), len(renderJobIDs))
	}(op.ID, projectID)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"op_id":  op.ID,
		"status": op.Status,
	})
}
