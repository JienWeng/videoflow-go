package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"videoflow-go/internal/presets"

	"github.com/google/uuid"
	"videoflow-go/internal/media"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

// operation preserves the existing synchronous/background API contract and records failures.
func (s *Server) operation(w http.ResponseWriter, r *http.Request, op *models.Op, work func(context.Context) (any, error)) {
	if r.URL.Query().Get("background") != "true" {
		result, err := work(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, 200, result)
		return
	}
	select {
	case s.operationSlots <- struct{}{}:
	default:
		http.Error(w, "Three background operations are already running. Wait for Activity before starting another.", 429)
		return
	}
	op.Status = "running"
	if err := s.database.CreateOp(op); err != nil {
		<-s.operationSlots
		http.Error(w, err.Error(), 500)
		return
	}
	s.tasks.Add(1)
	go func() {
		defer s.tasks.Done()
		defer func() { <-s.operationSlots }()
		timeout := 10 * time.Minute
		if s.cfg.UseLocalText {
			timeout = 45 * time.Minute
		}
		ctx, cancel := context.WithTimeout(s.ctx, timeout)
		if binding := r.Context().Value(directorBindingKey{}); binding != nil {
			ctx = context.WithValue(ctx, directorBindingKey{}, binding)
		}
		defer cancel()
		result, err := work(ctx)
		status := "succeeded"
		var errorText *string
		if err != nil {
			status = "failed"
			message := err.Error()
			errorText = &message
		}
		raw, _ := json.Marshal(result)
		if err := s.database.UpdateOp(op.ID, status, raw, errorText); err != nil {
			slog.Error("could not record generation completion", "operation_id", op.ID)
			return
		}
		s.broker.Publish(map[string]any{"type": "op_done", "op_id": op.ID, "kind": op.Kind, "status": status, "scene_id": op.SceneID, "output_id": op.OutputID, "error": errorText})
	}()
	writeJSON(w, 202, map[string]any{"op_id": op.ID, "status": "running"})
}

func (s *Server) agentJSON(ctx context.Context, agent, system, prompt string, images []string, target any) error {
	if len(images) == 0 && s.cfg.UseLocalText {
		return s.localTextJSON(ctx, agent, system, prompt, target)
	}
	client, model, err := s.routes().Agent(ctx, agent)
	if err != nil {
		return err
	}
	if len(images) > 0 {
		if _, err = client.Model(ctx, model, "vision"); err != nil {
			return err
		}
	}
	return client.StructuredVision(ctx, model, system, prompt, images, target)
}

// storagePath maps public storage URLs and legacy paths to the configured storage root.
func (s *Server) storagePath(path string) (string, error) {
	return media.StoragePath(s.cfg.StorageRoot, path)
}
func (s *Server) imageURL(asset *models.Asset) (string, error) {
	return media.ImageDataURL(s.cfg.StorageRoot, asset.FilePath)
}

func (s *Server) stylePrompt(projectID *string) string {
	if projectID != nil {
		if style, err := s.database.GetStyleGuide(*projectID); err == nil {
			return styleDirection(style)
		}
	}
	return defaultStyleGuide("").StylePrompt
}

func (s *Server) generateAssets(ctx context.Context, projectID *string, characterID *string, sceneID, name, kind, prompt, aspect string, references []string) ([]models.Asset, error) {
	descriptionPrompt := prompt
	if notes, ok := ctx.Value(storyboardShotsKey{}).(string); ok {
		prompt += notes
	}
	prompt += presets.Prompt(presets.From(ctx), "image")
	sceneRevision := ""
	if sceneID != "" {
		if frozen, ok := ctx.Value(sceneInputKey{}).(sceneInputSnapshot); ok && frozen.ID == sceneID {
			sceneRevision = frozen.Revision
		} else {
			var e error
			sceneRevision, e = s.currentSceneInputRevision(sceneID)
			if e != nil {
				return nil, e
			}
		}
	}
	ctx, frozenIdentity, err := s.directorGenerationContext(ctx, projectID, sceneID, characterID)
	if err != nil {
		return nil, err
	}
	references, identityNotes, err := s.directorImageInputs(ctx, projectID, references)
	if err != nil {
		return nil, err
	}
	prompt += identityNotes
	modelKind := "image"
	if characterID != nil || kind == "prop" {
		modelKind = "reference"
	}
	if !strings.Contains(prompt, "Visual style:") {
		direction := s.stylePrompt(projectID)
		if sceneID != "" {
			if scene, e := s.database.GetScene(sceneID); e == nil {
				direction = s.sceneVisualStyle(scene)
			}
		}
		prompt += "\nVisual style: " + direction
	}
	var images []providers.GeneratedImage
	provider, model := "codex", "native-image-generation"
	fallbackReason := ""
	fallback := true
	if s.cfg.UseLocalText {
		images, fallback, fallbackReason, err = s.codexImage(ctx, prompt, aspect, references)
		if err != nil {
			return nil, err
		}
	}
	if fallback {
		client, apiProvider, apiModel, routeErr := s.selectedMedia(ctx, modelKind)
		if routeErr != nil {
			return nil, routeErr
		}
		provider, model = apiProvider, apiModel
		if err != nil {
			return nil, err
		}
		payload := map[string]any{"model": model, "prompt": prompt}
		if aspect != "" {
			payload["aspect_ratio"] = aspect
		}
		if len(references) > 0 {
			refs := []map[string]any{}
			for _, image := range references {
				refs = append(refs, map[string]any{"type": "image_url", "image_url": map[string]string{"url": image}})
			}
			payload["input_references"] = refs
		}
		if err = client.ValidateImage(ctx, payload); err != nil {
			return nil, err
		}
		if provider != "openai" {
			var prepared struct {
				Prompt string `json:"prompt"`
			}
			if err = s.agentJSON(ctx, "prompt_agent", "Write a precise image-generation prompt from the brief. Preserve the characters, reference consistency, composition and visual style. Return prompt only as JSON.", prompt, nil, &prepared); err != nil {
				return nil, err
			}
			if strings.TrimSpace(prepared.Prompt) == "" {
				return nil, fmt.Errorf("Text provider returned no image prompt")
			}
			payload["prompt"] = prepared.Prompt + identityNotes
		}
		images, err = client.GenerateImages(ctx, payload)
		if err != nil {
			return nil, err
		}

	}
	assets := []models.Asset{}
	for _, image := range images {
		id := "asset_" + uuid.New().String()[:8]
		ext := ""
		switch image.MediaType {
		case "image/png":
			ext = ".png"
		case "image/jpeg":
			ext = ".jpg"
		case "image/webp":
			ext = ".webp"
		case "image/svg+xml":
			ext = ".svg"
		default:
			if extensions, _ := mime.ExtensionsByType(image.MediaType); len(extensions) > 0 {
				ext = extensions[0]
			}
		}
		if ext == "" {
			return nil, fmt.Errorf("unsupported generated media type %q", image.MediaType)
		}
		path := filepath.Join(s.cfg.StorageRoot, "assets", id+ext)
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
		if err = os.WriteFile(path, image.Data, 0644); err != nil {
			return nil, err
		}
		metadata, _ := json.Marshal(map[string]any{"scene_id": sceneID, "provider": provider, "model": model, "media_type": image.MediaType, "fallback_reason": fallbackReason, "scene_input_revision": sceneRevision, "creative_preset": presets.From(ctx)})
		if len(image.Usage) > 0 && string(image.Usage) != "null" {
			var fields map[string]any
			_ = json.Unmarshal(metadata, &fields)
			fields["usage"] = image.Usage
			metadata, _ = json.Marshal(fields)
		}
		if frozenIdentity != nil {
			var fields map[string]any
			_ = json.Unmarshal(metadata, &fields)
			fields["director_context_revision"] = frozenIdentity.Revision
			fields["director_context"] = frozenIdentity
			metadata, _ = json.Marshal(fields)
		}
		if s.cfg.UseLocalText || ctx.Value(imageReviewKey{}) == true {
			var reviewMeta map[string]any
			_ = json.Unmarshal(metadata, &reviewMeta)
			reviewMeta["consistency_review"] = "ready"
			metadata, _ = json.Marshal(reviewMeta)
		}
		asset := models.Asset{ID: id, ProjectID: projectID, CharacterID: characterID, Type: kind, Name: name, Description: descriptionPrompt, FilePath: filepath.ToSlash(filepath.Join("storage", "assets", id+ext)), TagsJSON: json.RawMessage(`[]`), MetadataJSON: metadata}
		if err = s.database.CreateAsset(&asset); err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	if projectID != nil {
		_ = s.database.SyncCosts(*projectID)
	}
	return assets, nil
}

type plannedAsset struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	Description     string `json:"description"`
	AssetID         string `json:"asset_id,omitempty"`
	ExistingAssetID string `json:"existing_asset_id,omitempty"`
	Reuse           bool   `json:"reuse"`
	Reason          string `json:"reason"`
	AssetType       string `json:"asset_type"`
}
type assetPlan struct {
	SceneID        string         `json:"scene_id"`
	Assets         []plannedAsset `json:"assets"`
	SuggestedShots []string       `json:"suggested_shots"`
}

func (s *Server) planAssets(ctx context.Context, scene *models.Scene, instruction string, maxAssets int) (*assetPlan, error) {
	if maxAssets <= 0 {
		maxAssets = 3
	}
	if maxAssets > 10 {
		maxAssets = 10
	}
	catalog, err := s.storyAssetCatalog(scene.ProjectID)
	if err != nil {
		return nil, err
	}
	catalogJSON, _ := json.Marshal(catalog)
	plan := &assetPlan{SuggestedShots: []string{}}
	err = s.agentJSON(ctx, "asset_planner", "Plan only essential visible props and locations, never characters or incidental objects. Return assets with name, type (prop or location), description, existing_asset_id (empty for new), reason, and suggested_shots. Inspect the supplied project library first. Reuse suitable existing assets by their exact ID, preserving appearance and locked traits. Explain every reuse or new asset decision. Do not invent IDs. Return an empty assets array when no references are needed.", fmt.Sprintf("Scene: %s\n%s\nInstructions: %s\nMaximum assets: %d\nProject library: %s", scene.Title, scene.Summary, instruction, maxAssets, catalogJSON), nil, plan)
	if err != nil {
		return nil, err
	}
	if plan.Assets == nil {
		return nil, fmt.Errorf("asset planner did not return an assets array")
	}
	if len(plan.Assets) > maxAssets {
		plan.Assets = plan.Assets[:maxAssets]
	}
	for i := range plan.Assets {
		asset := &plan.Assets[i]
		asset.AssetType = asset.Type
		if asset.ExistingAssetID == "" {
			asset.ExistingAssetID = matchingStoryAsset(*asset, catalog)
		}
		if asset.ExistingAssetID != "" {
			valid := false
			for _, candidate := range catalog {
				if candidate.ID == asset.ExistingAssetID && candidate.Type == asset.Type {
					valid = true
				}
			}
			if !valid {
				return nil, fmt.Errorf("asset reuse target is unavailable or incompatible")
			}
			asset.Reuse = true
		}
		if strings.TrimSpace(asset.Name) == "" || strings.TrimSpace(asset.Description) == "" || (asset.Type != "prop" && asset.Type != "location") {
			return nil, fmt.Errorf("OpenRouter returned an invalid asset plan")
		}
	}
	plan.SceneID = scene.ID
	return plan, nil
}
