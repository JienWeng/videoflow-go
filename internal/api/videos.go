package api

import (
	"context"
	"encoding/json"
	"fmt"
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

	project, _ := s.activeProjectID(r.Context())
	brief := generationBrief{Idea: "Preflight", AspectRatio: aspectRatio, Style: style, PresetID: r.URL.Query().Get("style_preset_id")}
	brief.SceneCount, _ = strconv.Atoi(r.URL.Query().Get("scene_count"))
	brief.CreateCharacters, _ = strconv.ParseBool(r.URL.Query().Get("create_characters"))
	brief.TargetDuration, _ = strconv.Atoi(r.URL.Query().Get("target_duration"))
	if ids := r.URL.Query().Get("character_ids"); ids != "" {
		brief.CharacterIDs = strings.Split(ids, ",")
	}
	if err := s.prepareBrief(project, &brief); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	missing := s.generationPrerequisites(r.Context(), brief)
	_, hasStyle := s.effectiveStyle(project, style)
	effective := brief.Style
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
		"ready":            len(missing) == 0,
		"automatic_assets": true, "character_references": true,
		"native_images":   s.preferCodexImages(r.Context()),
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
	project, err := s.activeProjectID(r.Context())
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
	routes, err := s.captureMediaRoutes(r.Context())
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
	op := &models.Op{Kind: "video_generation", Status: models.StatusRunning, ProjectID: &project}
	if err = s.database.CreateOp(op); err != nil {
		<-s.operationSlots
		http.Error(w, "failed to save operation", 500)
		return
	}
	s.tasks.Add(1)
	go func() {
		defer s.tasks.Done()
		defer func() { <-s.operationSlots }()
		ctx, cancel := context.WithTimeout(s.ctx, 45*time.Minute)
		defer cancel()
		if binding := r.Context().Value(directorBindingKey{}); binding != nil {
			ctx = context.WithValue(ctx, directorBindingKey{}, binding)
		}
		ctx = guidedStoryContext(ctx, project, brief.CharacterIDs)
		ctx = context.WithValue(brief.context(ctx), mediaRoutesKey{}, routes)
		stages := []string{}
		createdCharacters := []string{}
		savedScriptID := ""
		savedSceneIDs := []string{}
		cast := []string{}
		characterReferences := []string{}
		storyAssets := []string{}
		assetDecisions := []plannedAsset{}
		fail := func(err error) {
			message := err.Error()
			raw, _ := json.Marshal(map[string]any{"stages": stages, "brief": brief, "created_character_ids": createdCharacters, "script_id": savedScriptID, "scene_ids": savedSceneIDs, "project_id": project, "cast_character_ids": cast, "character_reference_ids": characterReferences, "story_asset_ids": storyAssets, "story_assets": assetDecisions, "automatic_assets": true, "character_references": true, "media_routes": routes})
			s.database.UpdateOp(op.ID, models.StatusFailed, raw, &message)
		}
		record := func(stage string) {
			stages = append(stages, stage)
			raw, _ := json.Marshal(map[string]any{"stages": stages, "brief": brief, "media_routes": routes, "created_character_ids": createdCharacters, "script_id": savedScriptID, "scene_ids": savedSceneIDs, "project_id": project, "cast_character_ids": cast, "character_reference_ids": characterReferences, "story_asset_ids": storyAssets, "story_assets": assetDecisions, "automatic_assets": true, "character_references": true})
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
		// Persisted scene/speaker references now use real project character IDs.
		savedScriptID = script.ID
		cast = []string{}
		seenCast := map[string]bool{}
		for i := range scenes {
			scene := &scenes[i]
			savedSceneIDs = append(savedSceneIDs, scene.ID)
			var ids []string
			json.Unmarshal(scene.CharacterIDsJSON, &ids)
			for _, id := range ids {
				if !seenCast[id] {
					cast = append(cast, id)
					seenCast[id] = true
				}
			}
		}
		originalCast := map[string]bool{}
		for _, id := range brief.CharacterIDs {
			originalCast[id] = true
		}
		for _, id := range cast {
			if !originalCast[id] {
				createdCharacters = append(createdCharacters, id)
			}
		}
		record("story")
		record("scenes")
		for i := range scenes {
			scene := &scenes[i]
			localBrief := sceneBrief(scene)
			sceneCtx := localBrief.context(guidedStoryContext(ctx, project, localBrief.CharacterIDs))
			drafts, err := s.agents.GenerateShots(sceneCtx, scene.Title, scene.Summary+"\n"+localBrief.prompt(), scene.Duration)
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

		var approved bool
		for i := range scenes {
			scene := &scenes[i]
			plan, planErr := s.planAssets(ctx, scene, brief.Style, 3)
			if planErr != nil {
				fail(planErr)
				return
			}
			assets, assetErr := s.materializeStoryAssets(ctx, scene, plan)
			assetDecisions = append(assetDecisions, plan.Assets...)
			for _, asset := range assets {
				storyAssets = append(storyAssets, asset.ID)
			}
			if assetErr != nil {
				fail(assetErr)
				return
			}
		}
		record("assets")
		style := brief.Style
		characterReferences, approved, err = s.storyCharacterReferences(ctx, project, cast, style)
		if err != nil {
			fail(err)
			return
		}
		record("characters")
		if !approved {
			fail(fmt.Errorf("story references are unavailable or excluded"))
			return
		}
		storyboardsMade, err := s.storyboardStory(ctx, scenes)
		if err != nil {
			fail(err)
			return
		}

		record("visuals")
		sceneIDs := []string{}
		for i := range scenes {
			scene := &scenes[i]
			sceneIDs = append(sceneIDs, scene.ID)
		}
		raw, _ := json.Marshal(map[string]any{"project_id": project, "script_id": script.ID, "scene_ids": sceneIDs, "stages": stages, "status": "ready_to_render", "storyboards_made": storyboardsMade, "cast_character_ids": cast, "character_reference_ids": characterReferences, "story_asset_ids": storyAssets, "story_assets": assetDecisions, "automatic_assets": true, "character_references": true, "brief": brief, "media_routes": routes, "created_character_ids": createdCharacters})
		if err = s.database.UpdateOp(op.ID, "succeeded", raw, nil); err != nil {
			fail(err)
		}
	}()
	writeJSON(w, 202, map[string]any{"op_id": op.ID, "status": models.StatusRunning, "automatic_assets": true, "character_references": true})
}
