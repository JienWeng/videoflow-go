package api

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
)

func (s *Server) directorGenerationContext(ctx context.Context, project *string, sceneID string, character *string) (context.Context, *directorContext, error) {
	if !s.cfg.UseLocalText {
		return ctx, nil, nil
	}
	if project == nil {
		return ctx, nil, fmt.Errorf("production project is required")
	}
	binding, _ := ctx.Value(directorBindingKey{}).(directorBinding)
	if binding.ProjectID == "" {
		binding.ProjectID = *project
	}
	if binding.ProjectID != *project {
		return ctx, nil, fmt.Errorf("production context belongs to another project")
	}
	if sceneID != "" {
		scene, err := s.database.GetScene(sceneID)
		if err != nil || scene.ProjectID == nil || *scene.ProjectID != *project {
			return ctx, nil, fmt.Errorf("scene not in production project")
		}
		var ids []string
		if json.Unmarshal(scene.CharacterIDsJSON, &ids) != nil {
			return ctx, nil, fmt.Errorf("invalid scene cast")
		}
		binding.Selection.CharacterIDs = append(binding.Selection.CharacterIDs, ids...)
	}
	if character != nil {
		binding.Selection.CharacterIDs = append(binding.Selection.CharacterIDs, *character)
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, id := range binding.Selection.CharacterIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	binding.Selection.CharacterIDs = ids
	ctx = context.WithValue(ctx, directorBindingKey{}, binding)
	snapshot, err := s.buildDirectorContext(*project, binding.Selection)
	if err != nil {
		return ctx, nil, err
	}
	ctx = context.WithValue(ctx, directorSnapshotKey{}, snapshot)
	return ctx, snapshot, nil
}
func imageAsset(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		return true
	}
	return false
}

// freezeDirectorVideo snapshots selected identity and voice policy before a job is durable/queued.
func (s *Server) freezeDirectorVideo(ctx context.Context, job *models.RenderJob) error {
	if !s.cfg.UseLocalText {
		return nil
	}
	sceneID := ""
	if job.SceneID != nil {
		sceneID = *job.SceneID
	}
	bound, snapshot, err := s.directorGenerationContext(ctx, job.ProjectID, sceneID, nil)
	if err != nil {
		return err
	}
	visual := []models.Asset{}
	for _, item := range snapshot.Assets {
		asset, err := s.database.GetAsset(item.ID)
		if err != nil {
			return err
		}
		if !models.ReferenceApproved(asset.MetadataJSON) {
			return fmt.Errorf("reference %s requires consistency review in Assets before video generation", asset.Name)
		}
		if !item.Available {
			return fmt.Errorf("selected reference %s is unavailable", asset.Name)
		}
		if imageAsset(asset.FilePath) {
			visual = append(visual, *asset)
		}
	}
	payload := map[string]any{}
	if len(job.RequestJSON) > 0 {
		if json.Unmarshal(job.RequestJSON, &payload) != nil {
			return fmt.Errorf("invalid video request")
		}
	}
	var keyframe *models.Asset
	unreviewed := false
	if sceneID != "" {
		scene, _ := s.database.GetScene(sceneID)
		var ids []string
		json.Unmarshal(scene.AssetIDsJSON, &ids)
		for _, id := range ids {
			asset, e := s.database.GetAsset(id)
			if e != nil || asset.Type != "storyboard" {
				continue
			}
			if !models.ReferenceApproved(asset.MetadataJSON) {
				unreviewed = true
				continue
			}
			var metadata struct {
				Revision string `json:"director_context_revision"`
			}
			json.Unmarshal(asset.MetadataJSON, &metadata)
			if metadata.Revision == snapshot.Revision || (metadata.Revision == "" && len(visual) == 0) {
				keyframe = asset
				break
			}
		}
		if keyframe == nil && unreviewed {
			return fmt.Errorf("generated keyframes await consistency review; approve a matching reference in Assets before Render")
		}
	}
	if keyframe == nil && len(visual) == 1 {
		keyframe = &visual[0]
	}
	if len(visual) > 0 && keyframe == nil {
		return fmt.Errorf("selected identities require a reviewed combined keyframe; generate and approve it before video rendering")
	}
	if keyframe != nil {
		if payload["frame_images"] != nil || payload["input_references"] != nil {
			return fmt.Errorf("manual frame overrides conflict with selected identity references; review the selection first")
		}
		client, _, err := s.routes().Media(job.Provider, job.Model, job.ProviderEndpoint)
		if err != nil {
			return err
		}
		model, err := client.Model(ctx, job.Model, "video")
		if err != nil {
			return err
		}
		supports := false
		for _, frame := range model.FrameImages {
			supports = supports || frame == "first_frame"
		}
		if !supports {
			return fmt.Errorf("selected video model cannot accept the reviewed identity keyframe")
		}
		image, err := s.imageURL(keyframe)
		if err != nil {
			return err
		}
		payload["frame_images"] = []map[string]any{{"type": "image_url", "image_url": map[string]string{"url": image}, "frame_type": "first_frame"}}
		payload["director_keyframe"] = keyframe.ID
	}
	if stringPrompt(payload["prompt"]) == "" {
		if job.ShotID != nil {
			shot, e := s.database.GetShot(*job.ShotID)
			if e != nil {
				return e
			}
			payload["prompt"] = shot.Prompt
		} else if sceneID != "" {
			scene, _ := s.database.GetScene(sceneID)
			prompt := scene.Title + ". " + scene.Summary
			shots, _ := s.database.ListShots(sceneID)
			for _, shot := range shots {
				prompt += fmt.Sprintf("\nShot %d (%ds): %s", shot.ShotOrder, shot.Duration, shot.Prompt)
			}
			payload["prompt"] = prompt
		}
	}
	payload["director_context"] = snapshot
	payload["prompt"] = stringPrompt(payload["prompt"]) + "\nAuthoritative captured identity context; preserve locked traits and reference roles: " + string(voiceJSON(snapshot))
	job.RequestJSON = voiceJSON(payload)
	return s.applyDirectorVoicePolicy(bound, job)
}
func (s *Server) prepareDirectorVideoJob(ctx context.Context, job *models.RenderJob) error {
	if err := s.prepareVideoJob(job); err != nil {
		return err
	}
	return s.freezeDirectorVideo(ctx, job)
}

// Called again at the provider submission boundary, including after a backend restart.
func (s *Server) validateFrozenVideo(ctx context.Context, job *models.RenderJob) error {
	var payload struct {
		Context  *directorContext `json:"director_context"`
		Keyframe string           `json:"director_keyframe"`
		Voice    struct {
			Bindings []db.VoiceBinding `json:"source_bindings"`
		} `json:"voice_policy"`
	}
	if json.Unmarshal(job.RequestJSON, &payload) != nil {
		return fmt.Errorf("invalid frozen render request")
	}
	if payload.Context == nil && len(payload.Voice.Bindings) == 0 {
		return nil
	}
	if job.ProjectID == nil {
		return fmt.Errorf("missing frozen render project")
	}
	if payload.Context != nil {
		if payload.Context.ProjectID != *job.ProjectID {
			return fmt.Errorf("frozen identity project mismatch")
		}
		selection := directorSelection{}
		for _, c := range payload.Context.Characters {
			selection.CharacterIDs = append(selection.CharacterIDs, c.ID)
		}
		for _, a := range payload.Context.Assets {
			selection.AssetIDs = append(selection.AssetIDs, a.ID)
		}
		if payload.Context.Selection != nil {
			selection = *payload.Context.Selection
		}
		current, err := s.buildDirectorContext(*job.ProjectID, selection)
		if err != nil || current.Revision != payload.Context.Revision {
			return fmt.Errorf("identity changed after render approval; review and prepare a new render")
		}
	}
	if payload.Keyframe != "" {
		asset, err := s.database.GetAsset(payload.Keyframe)
		if err != nil || asset.ProjectID == nil || *asset.ProjectID != *job.ProjectID || !models.ReferenceApproved(asset.MetadataJSON) {
			return fmt.Errorf("render keyframe is unavailable or no longer approved")
		}
	}
	if len(payload.Voice.Bindings) > 0 {
		if err := s.checkVoiceBindings(&db.VoiceTask{ProjectID: *job.ProjectID, Bindings: payload.Voice.Bindings}); err != nil {
			return err
		}
	}
	return nil
}
