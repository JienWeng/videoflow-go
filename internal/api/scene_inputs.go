package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"videoflow-go/internal/models"
)

type sceneInputKey struct{}
type sceneInputSnapshot struct{ ID, Revision string }

func (s *Server) sceneInputRevision(scene *models.Scene) (string, error) {
	shots, err := s.database.ListShots(scene.ID)
	if err != nil {
		return "", err
	}
	shotInputs := []any{}
	for _, shot := range shots {
		shotInputs = append(shotInputs, map[string]any{"order": shot.ShotOrder, "duration": shot.Duration, "prompt": shot.Prompt, "camera": shot.Camera, "movement": shot.Movement, "spec": shot.ShotJSON})
	}
	var document map[string]json.RawMessage
	_ = json.Unmarshal(scene.SceneJSON, &document)
	if document == nil {
		document = map[string]json.RawMessage{}
	}
	var brief map[string]json.RawMessage
	_ = json.Unmarshal(document["generation_brief"], &brief)
	if brief != nil {
		delete(brief, "character_notes")
		delete(brief, "character_ids")
		delete(brief, "reference_asset_ids")
		document["generation_brief"], _ = json.Marshal(brief)
	}
	delete(document, "story_asset_ids")
	style := s.stylePrompt(scene.ProjectID)
	if saved := sceneBrief(scene).CreativePreset; saved != nil {
		style = saved.Style
	}
	raw, err := json.Marshal(map[string]any{"title": scene.Title, "summary": scene.Summary, "duration": scene.Duration, "aspect": scene.AspectRatio, "scene": document, "shots": shotInputs, "style": style})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}
func (s *Server) bindSceneInputs(ctx context.Context, scene *models.Scene) (context.Context, error) {
	revision, err := s.sceneInputRevision(scene)
	return context.WithValue(ctx, sceneInputKey{}, sceneInputSnapshot{scene.ID, revision}), err
}
func (s *Server) currentSceneInputRevision(id string) (string, error) {
	scene, err := s.database.GetScene(id)
	if err != nil {
		return "", fmt.Errorf("scene is unavailable")
	}
	return s.sceneInputRevision(scene)
}
