package api

import (
	"context"
	"encoding/json"
	"fmt"
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
	if s.cfg.UseLocalText {
		session, err := s.directorSession(ctx)
		if err != nil {
			add("local_text", err.Error())
		} else if _, err = s.directorRunner.Available(session.Engine); err != nil {
			add("local_text", err.Error())
		} else if s.authManager().Status(ctx, session.Engine).SignedOut {
			name := "Codex"
			if session.Engine == "claude" {
				name = "Claude Code"
			}
			add("local_text", name+" is signed out, so it can't plan this story. Switch to a connected AI, or sign in again.")
		} else if session.Status != "ready" {
			add("local_text", "Local session is busy or requires review in Local sessions")
		}
	} else {
		for _, agent := range []string{"script_agent", "shot_agent", "prompt_agent"} {
			if _, _, err := s.routes().Agent(ctx, agent); err != nil {
				add(agent, err.Error())
			}
		}
	}
	needsSheets := s.cfg.UseLocalText && brief.CreateCharacters
	if s.cfg.UseLocalText {
		for _, id := range brief.CharacterIDs {
			c, e := s.database.GetCharacter(id)
			if e != nil {
				continue
			}
			var refs []string
			json.Unmarshal(c.ReferenceAssetIDsJSON, &refs)
			needsSheets = needsSheets || len(refs) == 0
		}
	}
	nativeImages := s.preferCodexImages(ctx)
	if needsSheets && !nativeImages {
		client, _, model, e := s.selectedMedia(ctx, "reference")
		if e != nil {
			add("reference", e.Error())
		} else if e = client.ValidateImage(ctx, map[string]any{"model": model, "prompt": "preflight", "aspect_ratio": "1:1"}); e != nil {
			add("reference", e.Error())
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
			json.Unmarshal(image.SupportedParameters, &caps)
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
