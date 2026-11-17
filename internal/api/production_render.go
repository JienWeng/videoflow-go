package api

import (
	"context"
	"encoding/json"
	"fmt"
	"videoflow-go/internal/models"
)

// Called only by a confirmed render action while holding the scene render lock.
func (s *Server) renderProductionPlan(ctx context.Context, scene *models.Scene, plan productionPlan, newVersion bool) (any, error) {
	if err := checkProductionSpend(ctx, plan, newVersion); err != nil {
		return nil, err
	}
	if plan.Blocked != "" {
		return nil, fmt.Errorf("%s", plan.Blocked)
	}
	if !plan.ReferencesReady {
		return nil, fmt.Errorf("resolve missing or excluded references before rendering")
	}
	reuseOnly := true
	reuseIDs := []string{}
	for _, u := range plan.Units {
		if u.JobID == "" || (newVersion && u.Status == "succeeded") {
			reuseOnly = false
		}
		if u.JobID != "" {
			reuseIDs = append(reuseIDs, u.JobID)
		}
	}
	if reuseOnly && len(reuseIDs) > 0 {
		return map[string]any{"job_id": reuseIDs[0], "job_ids": reuseIDs, "scene_id": scene.ID, "render_plan_id": plan.ID, "reused": true, "reused_count": len(reuseIDs), "new_calls": 0, "audio_mode": plan.Settings.AudioMode, "composition_url": "/compositions?scene_id=" + scene.ID}, nil
	}
	ctx = guidedStoryContext(ctx, *scene.ProjectID, nil)
	// Validate route and voice choice before spending storyboard quota.
	preflight := sceneRenderJob(*scene.ProjectID, scene)
	preflight.RequestJSON = voiceJSON(map[string]any{"audio_mode": plan.Settings.AudioMode, "music_prompt": plan.Settings.MusicPrompt})
	if err := s.prepareVideoJob(preflight); err != nil {
		return nil, err
	}
	if err := s.applyDirectorVoicePolicy(ctx, preflight); err != nil {
		return nil, err
	}
	updated := false
	if !plan.StoryboardReady {
		if _, err := s.generateMatchingStoryboard(ctx, scene); err != nil {
			return nil, fmt.Errorf("could not update the storyboard: %w", err)
		}
		updated = true
	}
	// Image preparation must not overwrite a newer scene/setting selection.
	current, err := s.database.GetScene(scene.ID)
	if err != nil {
		return nil, err
	}
	fresh, err := s.sceneProductionPlan(ctx, current)
	if err != nil {
		return nil, err
	}
	if fresh.ID != plan.ID {
		return nil, fmt.Errorf("scene inputs, model or production settings changed; reload the render plan")
	}
	plan = fresh
	if err := checkProductionSpend(ctx, plan, newVersion); err != nil {
		return nil, err
	}
	if plan.Blocked != "" {
		return nil, fmt.Errorf("%s", plan.Blocked)
	}
	pending := []*models.RenderJob{}
	ids := []string{}
	reused := 0
	for _, unit := range plan.Units {
		if unit.JobID != "" {
			old, e := s.database.GetRenderJob(unit.JobID)
			if e != nil {
				return nil, e
			}
			if old.Status != "pending" && old.Status != "running" && old.Status != "succeeded" {
				return nil, fmt.Errorf("clip state changed; recheck the render plan before retrying")
			}
			if old.Status != "succeeded" || !newVersion {
				ids = append(ids, old.ID)
				reused++
				continue
			}
		}
		job := sceneRenderJob(*scene.ProjectID, current)
		// Padding is generated to satisfy model minimums and trimmed during assembly.
		job.RequestJSON = voiceJSON(map[string]any{"duration": unit.Duration, "aspect_ratio": current.AspectRatio, "audio_mode": plan.Settings.AudioMode, "music_prompt": plan.Settings.MusicPrompt, "render_plan_id": plan.ID, "render_unit": unit, "prompt": current.Title + ". " + current.Summary + "\nRender only this consecutive part of the scene. Follow this local shot schedule:" + unit.Prompt})
		if err = s.prepareDirectorVideoJob(ctx, job); err != nil {
			return nil, err
		}
		pending = append(pending, job)
	}
	// Recheck after every unit has frozen its sources. Otherwise an edit during
	// freezing could produce a request from inputs the displayed plan never included.
	latest, e := s.database.GetScene(scene.ID)
	if e != nil {
		return nil, e
	}
	finalPlan, e := s.sceneProductionPlan(ctx, latest)
	if e != nil {
		return nil, e
	}
	if finalPlan.ID != plan.ID || finalPlan.Blocked != "" {
		return nil, fmt.Errorf("scene or clip state changed during preparation; recheck the plan")
	}
	if e = checkProductionSpend(ctx, finalPlan, newVersion); e != nil {
		return nil, e
	}
	// Persist all units or none; no partly validated batch can spend upstream.
	if err = s.database.CreateRenderJobs(pending); err != nil {
		return nil, err
	}
	for _, job := range pending {
		ids = append(ids, job.ID)
		s.workers.Enqueue(job.ID)
	}
	first := ""
	if len(ids) > 0 {
		first = ids[0]
	}
	return map[string]any{"job_id": first, "job_ids": ids, "scene_id": scene.ID, "render_plan_id": plan.ID, "status": "pending", "reused": len(pending) == 0, "reused_count": reused, "new_calls": len(pending), "storyboard_updated": updated, "audio_mode": plan.Settings.AudioMode, "composition_url": "/compositions?scene_id=" + scene.ID}, nil
}

func renderRequestUnit(raw json.RawMessage) renderUnit {
	var r struct {
		Unit renderUnit `json:"render_unit"`
	}
	_ = json.Unmarshal(raw, &r)
	return r.Unit
}

func (s *Server) lockSceneRender(id string) (func(), error) {
	value, _ := s.referenceLocks.LoadOrStore("scene-render:"+id, make(chan struct{}, 1))
	lock := value.(chan struct{})
	select {
	case lock <- struct{}{}:
		return func() { <-lock }, nil
	default:
		return nil, fmt.Errorf("scene render preparation is already running; check Activity")
	}
}

type confirmedRenderSpendKey struct{}
type productionSpendLimit struct{ Calls, Seconds, Storyboards int }

func productionSpend(plan productionPlan, newVersion bool) productionSpendLimit {
	result := productionSpendLimit{Calls: plan.Calls, Seconds: plan.NewSeconds}
	if newVersion {
		for _, u := range plan.Units {
			if u.Status == "succeeded" {
				result.Calls++
				result.Seconds += u.Duration
			}
		}
	}
	if result.Calls > 0 && !plan.StoryboardReady {
		result.Storyboards = 1
	}
	return result
}
func checkProductionSpend(ctx context.Context, plan productionPlan, newVersion bool) error {
	if approved, ok := ctx.Value(confirmedRenderSpendKey{}).(productionSpendLimit); ok {
		actual := productionSpend(plan, newVersion)
		if actual.Calls > approved.Calls || actual.Seconds > approved.Seconds || actual.Storyboards > approved.Storyboards {
			return fmt.Errorf("paid-call count increased after the plan was displayed; recheck the plan before generating")
		}
	}
	return nil
}
