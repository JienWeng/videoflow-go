package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"videoflow-go/internal/jobs"
	"videoflow-go/internal/models"
)

func (s *Server) directorGenerationContext(ctx context.Context, project *string, sceneID string, character *string) (context.Context, *directorContext, error) {
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
		binding.Selection = directorSelection{CharacterIDs: ids}
		binding.Selection.AssetIDs = append(binding.Selection.AssetIDs, s.sceneBoundAssets(scene)...)
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
			return fmt.Errorf("reference %s is excluded from generation; replace it in Library", asset.Name)
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
	if payload["video_input"] == inputReferences {
		// Reference mode: the connected pictures go as references and no storyboard is made.
		payload["director_context"] = snapshot
		if sceneRevision, err := s.currentSceneInputRevision(sceneID); err == nil && sceneID != "" {
			payload["scene_input_revision"] = sceneRevision
		}
		job.RequestJSON = voiceJSON(payload)
		return s.applyDirectorVoicePolicy(bound, job)
	}
	unreviewed := false
	hasSceneBoards := false
	sceneRevision := ""
	if sceneID != "" {
		sceneRevision, err = s.currentSceneInputRevision(sceneID)
		if err != nil {
			return err
		}
	}
	if sceneID != "" {
		scene, _ := s.database.GetScene(sceneID)
		ids := models.IDList(scene.AssetIDsJSON)
		for i := len(ids) - 1; i >= 0; i-- {
			id := ids[i]
			asset, e := s.database.GetAsset(id)
			if e != nil || asset.Type != "storyboard" {
				continue
			}
			hasSceneBoards = true
			var inputs struct {
				Revision string `json:"scene_input_revision"`
			}
			_ = json.Unmarshal(asset.MetadataJSON, &inputs) // unreadable metadata has no input revision
			if inputs.Revision != "" && inputs.Revision != sceneRevision {
				continue
			}
			if !models.ReferenceApproved(asset.MetadataJSON) {
				unreviewed = true
				continue
			}
			var metadata struct {
				Revision string `json:"director_context_revision"`
			}
			_ = json.Unmarshal(asset.MetadataJSON, &metadata) // unreadable metadata has no context revision
			if metadata.Revision == snapshot.Revision || (metadata.Revision == "" && len(visual) == 0) {
				keyframe = asset
				break
			}
		}
		if keyframe == nil && unreviewed {
			return fmt.Errorf("matching keyframes are excluded; generate a replacement before Render")
		}
	}
	if sceneID != "" && hasSceneBoards && keyframe == nil {
		return fmt.Errorf("scene inputs or references changed; regenerate a matching storyboard before rendering")
	}
	if sceneID == "" && keyframe == nil && len(visual) == 1 {
		keyframe = &visual[0]
	}
	if len(visual) > 0 && keyframe == nil {
		return fmt.Errorf("selected identities require a combined keyframe; generate it before video rendering")
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
		if sceneRevision != "" {
			payload["scene_input_revision"] = sceneRevision
		}
	}
	if stringPrompt(payload["prompt"]) == "" {
		if job.ShotID != nil {
			shot, e := s.database.GetShot(*job.ShotID)
			if e != nil {
				return e
			}
			payload["prompt"] = shot.Prompt
		} else if sceneID != "" {
			// Only a request saved before clips carried their own text gets here, when
			// it is retried.
			scene, e := s.database.GetScene(sceneID)
			if e != nil {
				return e
			}
			if payload["prompt"], e = jobs.ScenePrompt(s.database, scene); e != nil {
				return e
			}
		}
	}
	payload["director_context"] = snapshot
	payload["prompt"] = stringPrompt(payload["prompt"]) + "\nAuthoritative captured identity context; preserve locked traits and reference roles: " + string(voiceJSON(snapshot))
	job.RequestJSON = voiceJSON(payload)
	return s.applyDirectorVoicePolicy(bound, job)
}

// freezeReferencePictures attaches a reference-mode clip's pictures, checked against
// what the chosen model takes, before the request's sources are frozen.
func (s *Server) freezeReferencePictures(ctx context.Context, job *models.RenderJob) error {
	var payload map[string]any
	if json.Unmarshal(job.RequestJSON, &payload) != nil || payload["video_input"] != inputReferences {
		return nil
	}
	client, _, err := s.routes().Media(job.Provider, job.Model, job.ProviderEndpoint)
	if err != nil {
		return err
	}
	model, err := client.Model(ctx, job.Model, "video")
	if err != nil {
		return err
	}
	if model.ReferenceImages == 0 {
		return fmt.Errorf("the chosen video model cannot take reference pictures; choose first frame for this scene")
	}
	if err = s.attachReferencePictures(job, payload, model.ReferenceImages); err != nil {
		return err
	}
	job.RequestJSON = voiceJSON(payload)
	return nil
}

func (s *Server) prepareDirectorVideoJob(ctx context.Context, job *models.RenderJob) error {
	if err := s.prepareVideoJob(job); err != nil {
		return err
	}
	if err := s.freezeReferencePictures(ctx, job); err != nil {
		return err
	}
	if err := s.freezeDirectorVideo(ctx, job); err != nil {
		return err
	}
	var payload map[string]any
	if json.Unmarshal(job.RequestJSON, &payload) != nil || payload == nil {
		return fmt.Errorf("invalid frozen video request")
	}
	if job.SceneID != nil && payload["resolution"] == nil {
		if scene, e := s.database.GetScene(*job.SceneID); e == nil {
			resolution := sceneBrief(scene).Resolution
			if resolution != "" && resolution != "auto" {
				payload["resolution"] = resolution
			}
		}
	}
	if job.SceneID != nil {
		if scene, e := s.database.GetScene(*job.SceneID); e == nil {
			brief := sceneBrief(scene)
			direction := brief.creativeDirection()
			if brief.BriefVersion != 1 && brief.Style != "" {
				direction += "\nVisual style: " + brief.Style
			}
			if strings.TrimSpace(direction) != "" {
				payload["prompt"] = stringPrompt(payload["prompt"]) + "\n" + direction
			}
		}
	}
	delete(payload, "request_fingerprint")
	digest := sha256.Sum256(voiceJSON(map[string]any{"provider": job.Provider, "model": job.Model, "endpoint": job.ProviderEndpoint, "request": payload}))
	payload["request_fingerprint"] = hex.EncodeToString(digest[:])
	job.RequestJSON = voiceJSON(payload)
	return nil
}

// Called again at the provider submission boundary, including after a backend restart.
func (s *Server) validateFrozenVideo(ctx context.Context, job *models.RenderJob) error {
	var payload struct {
		SceneInput string           `json:"scene_input_revision"`
		Context    *directorContext `json:"director_context"`
		Keyframe   string           `json:"director_keyframe"`
	}
	if json.Unmarshal(job.RequestJSON, &payload) != nil {
		return fmt.Errorf("invalid frozen render request")
	}
	if payload.Context == nil {
		return nil
	}
	if job.ProjectID == nil {
		return fmt.Errorf("missing frozen render project")
	}
	if payload.SceneInput != "" && job.SceneID != nil {
		current, e := s.currentSceneInputRevision(*job.SceneID)
		if e != nil || current != payload.SceneInput {
			return fmt.Errorf("scene changed after render approval; review before submitting")
		}
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
	return nil
}
