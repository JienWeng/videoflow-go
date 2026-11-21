package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"videoflow-go/internal/models"
)

// applyDirectorVoicePolicy decides how a video request gets its sound, then writes the
// prompt of a planned scene clip from the scene's records.
func (s *Server) applyDirectorVoicePolicy(ctx context.Context, job *models.RenderJob) error {
	if err := s.voicePolicy(ctx, job); err != nil {
		return err
	}
	s.writeScenePrompt(ctx, job, true)
	return nil
}

func (s *Server) voicePolicy(ctx context.Context, job *models.RenderJob) error {
	payload := map[string]any{}
	if len(job.RequestJSON) > 0 {
		if err := json.Unmarshal(job.RequestJSON, &payload); err != nil || payload == nil {
			return fmt.Errorf("invalid video request for voice policy")
		}
	}
	if !s.cfg.UseLocalText && payload["audio_mode"] == nil {
		return nil
	}
	if job.ProjectID == nil {
		return fmt.Errorf("video voice policy requires a project")
	}
	binding, _ := ctx.Value(directorBindingKey{}).(directorBinding)
	if binding.ProjectID != "" && binding.ProjectID != *job.ProjectID {
		return fmt.Errorf("voice selection belongs to another project")
	}
	mode, _ := payload["conversation_mode"].(string)
	audioMode, _ := payload["audio_mode"].(string)
	music, _ := payload["music_prompt"].(string)

	var spoken []SpokenLine
	if job.SceneID != nil {
		scene, err := s.database.GetScene(*job.SceneID)
		if err != nil || scene.ProjectID == nil || *scene.ProjectID != *job.ProjectID {
			return fmt.Errorf("video scene does not belong to this project")
		}
		settings := sceneProduction(scene)
		if audioMode == "" {
			audioMode = settings.AudioMode
		}
		if music == "" {
			music = settings.MusicPrompt
		}
		brief := sceneBrief(scene)
		if brief.ConversationMode != "" {
			mode = brief.ConversationMode
		}
		var snapshot struct {
			Lines []SpokenLine `json:"spoken_lines"`
		}
		if err = json.Unmarshal(scene.SceneJSON, &snapshot); err != nil {
			return fmt.Errorf("scene spoken script is invalid")
		}
		spoken = snapshot.Lines
	}
	// Preserve the worker's deferred scene/shot prompt before adding policy text.
	if strings.TrimSpace(stringPrompt(payload["prompt"])) == "" {
		if job.ShotID != nil {
			shot, err := s.database.GetShot(*job.ShotID)
			if err != nil {
				return err
			}
			scene, err := s.database.GetScene(shot.SceneID)
			if err != nil || scene.ProjectID == nil || *scene.ProjectID != *job.ProjectID || (job.SceneID != nil && *job.SceneID != scene.ID) {
				return fmt.Errorf("shot voice context belongs to another scene/project")
			}
			payload["prompt"] = shot.Prompt
		}
		if strings.TrimSpace(stringPrompt(payload["prompt"])) == "" && job.SceneID != nil {
			scene, err := s.database.GetScene(*job.SceneID)
			if err != nil {
				return err
			}
			prompt := scene.Title + ". " + scene.Summary
			shots, err := s.database.ListShots(scene.ID)
			if err != nil {
				return err
			}
			for _, shot := range shots {
				prompt += "\n" + shot.Prompt
			}
			payload["prompt"] = prompt
		}
	}
	// Scenes saved by earlier versions may still ask for "saved" voices added
	// afterwards; that stage is gone, so such a clip is simply made without sound.
	audioMode = currentAudioMode(audioMode)
	if audioMode == "" {
		audioMode = "silent"
		if job.SceneID != nil {
			_, capabilities, e := s.productionVideoModel(ctx)
			if e == nil && capabilities.GenerateAudio {
				audioMode = "native"
			}
		}
		if mode == "silent" {
			audioMode = "silent"
		}
	}
	if audioMode != "native" && audioMode != "silent" {
		return fmt.Errorf("unsupported audio mode")
	}
	payload["audio_mode"] = audioMode
	if audioMode == "native" {
		_, capabilities, e := s.productionVideoModel(ctx)
		if e != nil || !capabilities.GenerateAudio {
			return fmt.Errorf("native audio is unavailable for the selected model; choose silent")
		}
		payload["generate_audio"] = true
		payload["voice_policy"] = map[string]any{"mode": "native", "project_id": *job.ProjectID}
		prompt := stringPrompt(payload["prompt"]) + "\nGenerate synchronized dialogue, ambience and sound effects. Native speaker identity and exact word/cut timing are best effort, not a saved-voice guarantee."
		if mode == "narration" {
			prompt += "\nUse off-screen narration rather than on-screen character dialogue."
		}
		if mode == "silent" {
			prompt += " No spoken dialogue or narration."
		}
		unit := renderRequestUnit(job.RequestJSON)
		if job.ShotID != nil && len(unit.ShotIDs) == 0 {
			unit.ShotIDs = []string{*job.ShotID}
		}
		settings := productionSettings{}
		if job.SceneID != nil {
			if scene, e := s.database.GetScene(*job.SceneID); e == nil {
				settings = sceneProduction(scene)
			}
		}
		for index, line := range spoken {
			if len(unit.ShotIDs) > 0 {
				id := line.ID
				if id == "" {
					id = fmt.Sprintf("line-%d", index)
				}
				target := settings.DialogueShots[id]
				if target != "" {
					included := false
					for _, shot := range unit.ShotIDs {
						included = included || shot == target
					}
					if !included {
						continue
					}
				}
				if target == "" && job.ShotID != nil && strings.TrimSpace(line.Text) != "" {
					return fmt.Errorf("assign dialogue to shots in the scene production settings before rendering an individual shot with native audio")
				}
			}
			if strings.TrimSpace(line.Text) != "" && mode != "silent" {
				speaker := line.CharacterID
				if speaker == "" && !line.Visible {
					speaker = "Narrator"
				} else if err := s.voiceCharacter(*job.ProjectID, speaker); err != nil {
					return fmt.Errorf("native dialogue speaker is not in this project")
				}
				prompt += fmt.Sprintf("\nSpeaker %s (%s): %s", speaker, line.Language, line.Text)
			}
		}
		if strings.TrimSpace(music) != "" {
			prompt += "\nBackground music direction: " + music + ". Keep music below dialogue."
		} else {
			prompt += "\nNo background music; keep dialogue, ambience and sound effects only."
		}
		payload["prompt"] = prompt
		job.RequestJSON = voiceJSON(payload)
		return nil
	}
	payload["generate_audio"] = false
	payload["voice_policy"] = map[string]any{"mode": "silent", "project_id": *job.ProjectID}
	payload["prompt"] = stringPrompt(payload["prompt"]) + "\nAuthoritative voice policy: SILENT visual pass. Generate no speech or audio."
	job.RequestJSON = voiceJSON(payload)
	return nil
}
func stringPrompt(value any) string { prompt, _ := value.(string); return prompt }
