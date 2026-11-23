package api

import (
	"context"
	"encoding/json"
	"fmt"
	"videoflow-go/internal/models"
)

func (s *Server) generationPrerequisites(ctx context.Context, brief generationBrief) []map[string]string {
	missing := []map[string]string{}
	add := func(route, reason string) {
		setting := "/settings"
		if route == "local_text" {
			setting = "/director"
		}
		missing = append(missing, map[string]string{"route": route, "setting": setting, "reason": reason})
	}
	if problem := s.localTextProblem(ctx, "plan this story"); problem != "" {
		add("local_text", problem)
	}
	needsSheets := brief.CreateCharacters
	for _, id := range brief.CharacterIDs {
		c, e := s.database.GetCharacter(id)
		if e != nil {
			continue
		}
		refs := models.IDList(c.ReferenceAssetIDsJSON)
		needsSheets = needsSheets || len(refs) == 0
	}
	nativeImages := s.preferCodexImages(ctx)
	if needsSheets && !nativeImages {
		if problem := s.referenceImageProblem(ctx); problem != "" {
			add("reference", problem)
		}
	}
	if !nativeImages {
		client, _, imageModel, err := s.selectedMedia(ctx, "image")
		if err != nil {
			add("image", err.Error())
			return missing
		}
		image, err := client.Model(ctx, imageModel, "image")
		if err != nil {
			add("image", err.Error())
		} else if len(brief.ReferenceAssetIDs) > 0 || needsSheets {
			var caps map[string]json.RawMessage
			_ = json.Unmarshal(image.SupportedParameters, &caps) // unreadable parameters mean no reference support
			if caps["input_references"] == nil {
				add("image", "Selected image model does not support character references; choose a reference-capable image model before generating")
			}
		}
		imagePayload := map[string]any{"model": imageModel, "prompt": "preflight", "aspect_ratio": brief.AspectRatio}
		if len(brief.ReferenceAssetIDs) > 0 || needsSheets {
			refs := []map[string]any{}
			for range max(1, len(brief.ReferenceAssetIDs)) {
				refs = append(refs, map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64,iVBORw0KGgo="}})
			}
			imagePayload["input_references"] = refs
		}
		if err := client.ValidateImage(ctx, imagePayload); err != nil {
			add("image", err.Error())
		}
	}
	videoClient, _, videoModel, err := s.selectedMedia(ctx, "video")
	if err != nil {
		add("video", err.Error())
		return missing
	}
	video, err := videoClient.Model(ctx, videoModel, "video")
	if err != nil {
		add("video", err.Error())
		return missing
	}
	supported := false
	for _, a := range video.AspectRatios {
		supported = supported || a == brief.AspectRatio
	}
	if !supported {
		add("video", "Selected video model does not support aspect ratio "+brief.AspectRatio)
	}
	if brief.SceneCount > 0 && brief.TargetDuration > 0 {
		max := 0
		for _, duration := range video.Durations {
			if duration > max {
				max = duration
			}
		}
		target := (brief.TargetDuration + brief.SceneCount - 1) / brief.SceneCount
		if target > max {
			need := (brief.TargetDuration + max - 1) / max
			add("duration", fmt.Sprintf("A %d-second video needs at least %d scenes, because this video model makes clips of up to %d seconds. Set Scenes to %d or more (step 3, Length & scenes), or shorten the video.", brief.TargetDuration, need, max, need))
		}
	}
	if len(brief.ReferenceAssetIDs) > 0 || needsSheets {
		conditioned := false
		for _, f := range video.FrameImages {
			conditioned = conditioned || f == "first_frame"
		}
		if !conditioned {
			add("video", "Selected video model cannot use the character-conditioned keyframe; select a first-frame model")
		}
	}
	return missing
}

// localTextProblem says why the selected local AI session cannot take a turn now, or ""
// when it can. task finishes "… is signed out, so it can't …".
func (s *Server) localTextProblem(ctx context.Context, task string) string {
	session, err := s.directorSession(ctx)
	if err != nil {
		return err.Error()
	}
	if _, err = s.directorRunner.Available(session.Engine); err != nil {
		return err.Error()
	}
	if s.authManager().Status(ctx, session.Engine).SignedOut {
		name := "Codex"
		if session.Engine == "claude" {
			name = "Claude Code"
		}
		return name + " is signed out, so it can't " + task + ". Switch to a connected AI, or sign in again."
	}
	if session.Status != "ready" {
		return "The AI session is busy with another request. Wait for it to finish, or stop it."
	}
	return ""
}

// referenceImageProblem says why the selected picture service cannot make a character
// sheet (a missing key, or a model that refuses a square picture), or "" when it can.
// It only reads the service's catalog; nothing is generated.
func (s *Server) referenceImageProblem(ctx context.Context) string {
	client, _, model, err := s.selectedMedia(ctx, "reference")
	if err != nil {
		return err.Error()
	}
	if err = client.ValidateImage(ctx, map[string]any{"model": model, "prompt": "preflight", "aspect_ratio": "1:1"}); err != nil {
		return err.Error()
	}
	return ""
}
