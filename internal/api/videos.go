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
	run := &videoGeneration{
		s: s, op: op, project: project, brief: brief, routes: routes,
		stages: []string{}, createdCharacters: []string{}, savedSceneIDs: []string{}, cast: []string{},
		characterReferences: []string{}, storyAssets: []string{}, assetDecisions: []plannedAsset{},
	}
	binding := r.Context().Value(directorBindingKey{})
	s.tasks.Add(1)
	go func() {
		defer s.tasks.Done()
		defer func() { <-s.operationSlots }()
		ctx, cancel := context.WithTimeout(s.ctx, 45*time.Minute)
		defer cancel()
		if binding != nil {
			ctx = context.WithValue(ctx, directorBindingKey{}, binding)
		}
		ctx = guidedStoryContext(ctx, project, brief.CharacterIDs)
		ctx = context.WithValue(brief.context(ctx), mediaRoutesKey{}, routes)
		if err := run.run(ctx); err != nil {
			run.fail(err)
		}
	}()
	s.recordBriefDecisions(project, brief)
	writeJSON(w, 202, map[string]any{"op_id": op.ID, "status": models.StatusRunning, "automatic_assets": true, "character_references": true})
}

// videoGeneration is one guided run from an idea to storyboarded scenes ready to
// render. It records what it has made on its operation after every stage, so a failed
// run still shows what was created.
type videoGeneration struct {
	s                   *Server
	op                  *models.Op
	project             string
	brief               generationBrief
	routes              map[string]mediaRoute
	stages              []string
	createdCharacters   []string
	savedScriptID       string
	savedSceneIDs       []string
	cast                []string
	characterReferences []string
	storyAssets         []string
	assetDecisions      []plannedAsset
}

func (g *videoGeneration) run(ctx context.Context) error {
	script, scenes, err := g.writeStory(ctx)
	if err != nil {
		return err
	}
	g.record("story")
	g.record("scenes")
	if err = g.writeShots(ctx, scenes); err != nil {
		return err
	}
	g.record("shots")
	if err = g.makeStoryAssets(ctx, scenes); err != nil {
		return err
	}
	g.record("assets")
	var approved bool
	g.characterReferences, approved, err = g.s.storyCharacterReferences(ctx, g.project, g.cast, g.brief.Style)
	if err != nil {
		return err
	}
	g.record("characters")
	if !approved {
		return fmt.Errorf("story references are unavailable or excluded")
	}
	storyboardsMade, err := g.s.storyboardStory(ctx, scenes)
	if err != nil {
		return err
	}
	g.record("visuals")
	return g.finish(script, scenes, storyboardsMade)
}

func (g *videoGeneration) fail(err error) {
	message := err.Error()
	raw, _ := json.Marshal(map[string]any{"stages": g.stages, "brief": g.brief, "created_character_ids": g.createdCharacters, "script_id": g.savedScriptID, "scene_ids": g.savedSceneIDs, "project_id": g.project, "cast_character_ids": g.cast, "character_reference_ids": g.characterReferences, "story_asset_ids": g.storyAssets, "story_assets": g.assetDecisions, "automatic_assets": true, "character_references": true, "media_routes": g.routes})
	g.s.database.UpdateOp(g.op.ID, models.StatusFailed, raw, &message)
}

func (g *videoGeneration) record(stage string) {
	g.stages = append(g.stages, stage)
	raw, _ := json.Marshal(map[string]any{"stages": g.stages, "brief": g.brief, "media_routes": g.routes, "created_character_ids": g.createdCharacters, "script_id": g.savedScriptID, "scene_ids": g.savedSceneIDs, "project_id": g.project, "cast_character_ids": g.cast, "character_reference_ids": g.characterReferences, "story_asset_ids": g.storyAssets, "story_assets": g.assetDecisions, "automatic_assets": true, "character_references": true})
	g.s.database.UpdateOp(g.op.ID, "running", raw, nil)
	g.s.broker.Publish(map[string]any{"type": "pipeline_stage", "op_id": g.op.ID, "stage": stage, "stages": g.stages, "project_id": g.project})
}

// writeStory drafts and saves the script and its scenes, then notes the cast, including
// characters the draft created.
func (g *videoGeneration) writeStory(ctx context.Context) (*models.Script, []models.Scene, error) {
	draft, err := g.s.agents.GenerateScript(ctx, g.brief.prompt(), g.brief.SceneCount, g.brief.TargetDuration, g.brief.AspectRatio)
	if err != nil {
		return nil, nil, err
	}
	script, scenes, err := g.s.persistDraft(g.project, g.brief, draft)
	if err != nil {
		return nil, nil, err
	}
	// Persisted scene/speaker references now use real project character IDs.
	g.savedScriptID = script.ID
	g.cast = []string{}
	seenCast := map[string]bool{}
	for i := range scenes {
		scene := &scenes[i]
		g.savedSceneIDs = append(g.savedSceneIDs, scene.ID)
		ids := models.IDList(scene.CharacterIDsJSON)
		for _, id := range ids {
			if !seenCast[id] {
				g.cast = append(g.cast, id)
				seenCast[id] = true
			}
		}
	}
	originalCast := map[string]bool{}
	for _, id := range g.brief.CharacterIDs {
		originalCast[id] = true
	}
	for _, id := range g.cast {
		if !originalCast[id] {
			g.createdCharacters = append(g.createdCharacters, id)
		}
	}
	return script, scenes, nil
}

// writeShots breaks each scene into shots.
func (g *videoGeneration) writeShots(ctx context.Context, scenes []models.Scene) error {
	for i := range scenes {
		scene := &scenes[i]
		localBrief := sceneBrief(scene)
		sceneCtx := localBrief.context(guidedStoryContext(ctx, g.project, localBrief.CharacterIDs))
		drafts, err := g.s.agents.GenerateShots(sceneCtx, scene.Title, scene.Summary+"\n"+localBrief.prompt(), scene.Duration)
		if err != nil {
			return err
		}
		shots := []models.Shot{}
		for _, d := range drafts {
			camera, movement := d.Camera, d.Movement
			shots = append(shots, models.Shot{ShotOrder: d.Order, Duration: d.Duration, Prompt: d.Prompt, Camera: &camera, Movement: &movement})
		}
		if err = g.s.database.ReplaceSceneShots(scene, shots); err != nil {
			return err
		}
	}
	return nil
}

// makeStoryAssets plans and makes each scene's places and props, keeping what was made
// even when a later one fails.
func (g *videoGeneration) makeStoryAssets(ctx context.Context, scenes []models.Scene) error {
	for i := range scenes {
		scene := &scenes[i]
		plan, planErr := g.s.planAssets(ctx, scene, g.brief.Style, 3)
		if planErr != nil {
			return planErr
		}
		assets, assetErr := g.s.materializeStoryAssets(ctx, scene, plan)
		g.assetDecisions = append(g.assetDecisions, plan.Assets...)
		for _, asset := range assets {
			g.storyAssets = append(g.storyAssets, asset.ID)
		}
		if assetErr != nil {
			return assetErr
		}
	}
	return nil
}

// finish marks the run ready to render.
func (g *videoGeneration) finish(script *models.Script, scenes []models.Scene, storyboardsMade int) error {
	sceneIDs := []string{}
	for i := range scenes {
		scene := &scenes[i]
		sceneIDs = append(sceneIDs, scene.ID)
	}
	raw, _ := json.Marshal(map[string]any{"project_id": g.project, "script_id": script.ID, "scene_ids": sceneIDs, "stages": g.stages, "status": "ready_to_render", "storyboards_made": storyboardsMade, "cast_character_ids": g.cast, "character_reference_ids": g.characterReferences, "story_asset_ids": g.storyAssets, "story_assets": g.assetDecisions, "automatic_assets": true, "character_references": true, "brief": g.brief, "media_routes": g.routes})
	return g.s.database.UpdateOp(g.op.ID, "succeeded", raw, nil)
}
