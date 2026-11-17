package api

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"videoflow-go/internal/models"
)

type storyboardShotsKey struct{}
type confirmedRenderPlanKey struct{}

func sceneRenderJob(project string, scene *models.Scene) *models.RenderJob {
	stage, progress := "submitted", "Whole scene render queued"
	return &models.RenderJob{ProjectID: &project, SceneID: &scene.ID, Status: "pending", Stage: &stage, Progress: &progress, RequestJSON: []byte(fmt.Sprintf(`{"scene_id":%q}`, scene.ID))}
}

// Called only by an explicit prepare_storyboard render request. Never on read.
func (s *Server) prepareAndRenderScene(ctx context.Context, project, id string, newVersion ...bool) (any, error) {
	unlock, err := s.lockSceneRender(id)
	if err != nil {
		return nil, err
	}
	defer unlock()
	scene, err := s.database.GetScene(id)
	if err != nil || scene.ProjectID == nil || *scene.ProjectID != project {
		return nil, fmt.Errorf("scene is not in this project")
	}
	shots, err := s.database.ListShots(id)
	if err != nil {
		return nil, err
	}
	if len(shots) == 0 {
		return nil, fmt.Errorf("generate or add shots before rendering this scene")
	}
	plan, e := s.sceneProductionPlan(ctx, scene)
	if e != nil {
		return nil, e
	}
	if expected, _ := ctx.Value(confirmedRenderPlanKey{}).(string); expected != "" && expected != plan.ID {
		return nil, fmt.Errorf("render plan changed before preparation; reload before generating")
	}
	return s.renderProductionPlan(ctx, scene, plan, len(newVersion) > 0 && newVersion[0])
}

func (s *Server) generateMatchingStoryboard(ctx context.Context, scene *models.Scene) (any, error) {
	ctx, err := s.bindSceneInputs(ctx, scene)
	if err != nil {
		return nil, err
	}
	refs, err := s.sceneReferences(scene)
	if err != nil {
		return nil, err
	}
	brief := sceneBrief(scene)
	if profile, e := s.database.GetStyleGuide(*scene.ProjectID); e == nil && profile.StylePrompt != "" {
		if brief.CreativePreset == nil {
			brief.Style = s.stylePrompt(scene.ProjectID)
		}
	}
	shotNotes, err := s.storyboardShotNotes(scene.ID)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, storyboardShotsKey{}, shotNotes)
	assets, err := s.generateAssets(brief.context(ctx), scene.ProjectID, nil, scene.ID, scene.Title+" storyboard", "storyboard", "Storyboard keyframe of "+scene.Title+". "+scene.Summary+"\n"+brief.prompt(), scene.AspectRatio, refs)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	_ = json.Unmarshal(scene.AssetIDsJSON, &ids)
	for _, asset := range assets {
		ids = append(ids, asset.ID)
	}
	scene.AssetIDsJSON, _ = json.Marshal(ids)
	if err = s.database.UpdateScene(scene); err != nil {
		return nil, err
	}
	return map[string]any{"scene_id": scene.ID, "asset_id": assets[0].ID, "storyboard_url": "/" + assets[0].FilePath}, nil
}

func (s *Server) storyboardShotNotes(id string) (string, error) {
	shots, err := s.database.ListShots(id)
	if err != nil {
		return "", err
	}
	notes := "\nCurrent shot plan:"
	for _, shot := range shots {
		notes += fmt.Sprintf("\nShot %d (%d seconds): %s", shot.ShotOrder, shot.Duration, shot.Prompt)
		if shot.Camera != nil {
			notes += " Camera: " + *shot.Camera
		}
		if shot.Movement != nil {
			notes += " Movement: " + *shot.Movement
		}
	}
	return notes, nil
}

// Call under the per-scene render lock. A second click must not spend again.
func (s *Server) existingSceneRender(job *models.RenderJob, newVersion bool) (*models.RenderJob, error) {
	jobs, err := s.database.ListRenderJobs(*job.ProjectID)
	if err != nil {
		return nil, err
	}
	var completed *models.RenderJob
	for i := range jobs {
		old := &jobs[i]
		if old.SceneID == nil || job.SceneID == nil || *old.SceneID != *job.SceneID {
			continue
		}
		active := old.Status == "pending" || old.Status == "running" || old.Status == "reconciliation_required"
		sameTarget := (old.ShotID == nil && job.ShotID == nil) || (old.ShotID != nil && job.ShotID != nil && *old.ShotID == *job.ShotID)
		if !sameTarget {
			if active {
				return nil, fmt.Errorf("another scene or shot render is active; check Videos before spending again")
			}
			continue
		}

		if !active && (old.Status != "succeeded" || newVersion) {
			continue
		}
		var a, b any
		if json.Unmarshal(old.RequestJSON, &a) != nil || json.Unmarshal(job.RequestJSON, &b) != nil {
			return nil, fmt.Errorf("cannot compare existing scene render; check Videos before submitting again")
		}
		same := reflect.DeepEqual(a, b)
		if am, ok := a.(map[string]any); ok {
			if bm, ok := b.(map[string]any); ok {
				af, _ := am["request_fingerprint"].(string)
				bf, _ := bm["request_fingerprint"].(string)
				if af != "" && bf != "" {
					same = af == bf
				}
			}
		}
		if old.Provider == job.Provider && old.Model == job.Model && old.ProviderEndpoint == job.ProviderEndpoint && same {
			if active {
				return old, nil
			}
			completed = old
			continue
		}
		if active {
			return nil, fmt.Errorf("this scene already has an active or unresolved render; check Videos before submitting another")
		}
	}
	return completed, nil
}
