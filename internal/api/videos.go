package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"videoflow-go/internal/models"
)

func (s *Server) handleVideoPreflight(w http.ResponseWriter, r *http.Request) {
	aspectRatio := r.URL.Query().Get("aspect_ratio")
	if aspectRatio == "" {
		aspectRatio = s.routes().Setting("default_aspect_ratio", "9:16")
	}
	style := r.URL.Query().Get("style")
	if style == "" {
		style = "2d-picture-book"
	}

	project, _ := s.database.GetActiveProjectID()
	brief := generationBrief{Idea: "Preflight", AspectRatio: aspectRatio, Style: style}
	brief.SceneCount, _ = strconv.Atoi(r.URL.Query().Get("scene_count"))
	brief.TargetDuration, _ = strconv.Atoi(r.URL.Query().Get("target_duration"))
	if ids := r.URL.Query().Get("character_ids"); ids != "" {
		brief.CharacterIDs = strings.Split(ids, ",")
	}
	if err := s.prepareBrief(project, &brief); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	missing := s.generationPrerequisites(r.Context(), brief)
	effective, hasStyle := s.effectiveStyle(project, style)
	client, videoProvider, videoModel, _ := s.routes().SelectedMedia("video")
	imageProvider, _ := s.routes().MediaSelection("image")
	var durations []int
	supportsReferences := false
	if client != nil {
		if model, err := client.Model(r.Context(), videoModel, "video"); err == nil {
			durations = model.Durations
			for _, f := range model.FrameImages {
				supportsReferences = supportsReferences || f == "first_frame"
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ready":           len(missing) == 0,
		"effective_style": effective, "has_project_style": hasStyle, "video_durations": durations, "supports_references": supportsReferences,
		"missing": missing,
		"effective_routes": map[string]string{
			"video_provider": videoProvider,
			"image_provider": imageProvider,
			"video_model":    videoModel,
			"image_model":    s.routes().MediaModel("image"),
		},
		"style": map[string]string{
			"selected": style,
			"aspect":   aspectRatio,
		},
	})
}

func (s *Server) handleGenerateVideo(w http.ResponseWriter, r *http.Request) {
	project, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	var brief generationBrief
	if json.NewDecoder(r.Body).Decode(&brief) != nil {
		http.Error(w, "invalid generation request", 400)
		return
	}
	if err = s.prepareBrief(project, &brief); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	routes, err := s.captureMediaRoutes()
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	preflightCtx := context.WithValue(r.Context(), mediaRoutesKey{}, routes)
	if missing := s.generationPrerequisites(preflightCtx, brief); len(missing) > 0 {
		http.Error(w, missing[0]["reason"], 400)
		return
	}
	select {
	case s.operationSlots <- struct{}{}:
	default:
		http.Error(w, "Three background operations are already running. Check Activity before starting another.", 429)
		return
	}
	op := &models.Op{Kind: "video_generation", Status: "running", ProjectID: &project}
	if err = s.database.CreateOp(op); err != nil {
		<-s.operationSlots
		http.Error(w, "failed to save operation", 500)
		return
	}
	s.tasks.Add(1)
	go func() {
		defer s.tasks.Done()
		defer func() { <-s.operationSlots }()
		ctx, cancel := context.WithTimeout(s.ctx, 10*time.Minute)
		defer cancel()
		ctx = context.WithValue(brief.context(ctx), mediaRoutesKey{}, routes)
		stages := []string{}
		fail := func(err error) { message := err.Error(); s.database.UpdateOp(op.ID, "failed", nil, &message) }
		record := func(stage string) {
			stages = append(stages, stage)
			raw, _ := json.Marshal(map[string]any{"stages": stages, "brief": brief, "media_routes": routes})
			s.database.UpdateOp(op.ID, "running", raw, nil)
			s.broker.Publish(map[string]any{"type": "pipeline_stage", "op_id": op.ID, "stage": stage, "stages": stages, "project_id": project})
		}
		draft, err := s.agents.GenerateScript(ctx, brief.prompt(), brief.SceneCount, brief.TargetDuration, brief.AspectRatio)
		if err != nil {
			fail(err)
			return
		}
		script, scenes, err := s.persistDraft(project, brief, draft)
		if err != nil {
			fail(err)
			return
		}
		record("story")
		record("scenes")
		for i := range scenes {
			scene := &scenes[i]
			drafts, err := s.agents.GenerateShots(ctx, scene.Title, scene.Summary+"\n"+brief.prompt(), scene.Duration)
			if err != nil {
				fail(err)
				return
			}
			shots := []models.Shot{}
			for _, d := range drafts {
				camera, movement := d.Camera, d.Movement
				shots = append(shots, models.Shot{ShotOrder: d.Order, Duration: d.Duration, Prompt: d.Prompt, Camera: &camera, Movement: &movement})
			}
			if err = s.database.ReplaceSceneShots(scene, shots); err != nil {
				fail(err)
				return
			}
		}
		record("shots")
		for i := range scenes {
			scene := &scenes[i]
			refs, err := s.sceneReferences(scene)
			if err != nil {
				fail(err)
				return
			}
			images, err := s.generateAssets(ctx, scene.ProjectID, nil, scene.ID, scene.Title+" keyframe", "storyboard", scene.Title+". "+scene.Summary+"\n"+brief.prompt(), scene.AspectRatio, refs)
			if err != nil {
				fail(err)
				return
			}
			var ids []string
			json.Unmarshal(scene.AssetIDsJSON, &ids)
			for _, image := range images {
				ids = append(ids, image.ID)
			}
			scene.AssetIDsJSON, _ = json.Marshal(ids)
			if err = s.database.UpdateScene(scene); err != nil {
				fail(err)
				return
			}
		}
		record("visuals")
		jobs := []models.RenderJob{}
		sceneIDs := []string{}
		for i := range scenes {
			scene := &scenes[i]
			stage, progress := "submitted", "Queued scene clip"
			jobs = append(jobs, models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Provider: routes["video"].Provider, Model: routes["video"].Model, ProviderEndpoint: routes["video"].Endpoint, Status: "pending", Stage: &stage, Progress: &progress})
			sceneIDs = append(sceneIDs, scene.ID)
		}
		stages = append(stages, "render")
		result := map[string]any{"project_id": project, "script_id": script.ID, "scene_ids": sceneIDs, "stages": stages, "status": "rendering", "brief": brief, "media_routes": routes}
		if err = s.database.SaveOperationJobs(op.ID, jobs, result); err != nil {
			fail(err)
			return
		}
		for _, job := range jobs {
			s.workers.Enqueue(job.ID)
		}
	}()
	writeJSON(w, 202, map[string]any{"op_id": op.ID, "status": "running"})
}
