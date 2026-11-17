package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
)

// applyDirectorVoicePolicy mutates the durable request before enqueue. Ordinary
// video is the visual pass; speech and visible-speaker lip sync remain explicit
// /voices stages. It never contacts a provider or replaces a native audio sample.
func (s *Server) applyDirectorVoicePolicy(ctx context.Context, job *models.RenderJob) error {
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

	characters := append([]string{}, binding.Selection.CharacterIDs...)
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
		var cast []string
		if err = json.Unmarshal(scene.CharacterIDsJSON, &cast); err != nil {
			return fmt.Errorf("scene cast is invalid")
		}
		characters = append(characters, cast...)
		characters = append(characters, brief.CharacterIDs...)
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
	if audioMode == "" {
		audioMode = "saved"
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
	if audioMode != "native" && audioMode != "saved" && audioMode != "silent" {
		return fmt.Errorf("unsupported audio mode")
	}
	payload["audio_mode"] = audioMode
	if audioMode == "native" {
		_, capabilities, e := s.productionVideoModel(ctx)
		if e != nil || !capabilities.GenerateAudio {
			return fmt.Errorf("native audio is unavailable for the selected model; choose saved voices or silent")
		}
		payload["generate_audio"] = true
		payload["voice_policy"] = map[string]any{"mode": "native", "project_id": *job.ProjectID, "exact_voice_identity": false}
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
	if audioMode == "silent" {
		payload["generate_audio"] = false
		payload["voice_policy"] = map[string]any{"mode": "silent", "project_id": *job.ProjectID}
		payload["prompt"] = stringPrompt(payload["prompt"]) + "\nAuthoritative voice policy: SILENT visual pass. Generate no speech or audio."
		job.RequestJSON = voiceJSON(payload)
		return nil
	}
	if len(spoken) > 0 {
		characters = nil
		for _, line := range spoken {
			if strings.TrimSpace(line.Text) != "" {
				if line.CharacterID == "" {
					return fmt.Errorf("spoken line has no character; set up its speaker in /voices")
				}
				characters = append(characters, line.CharacterID)
			}
		}
	}
	seen := map[string]bool{}
	bindings := []db.VoiceBinding{}
	speakers := []map[string]any{}
	for _, id := range characters {
		if seen[id] {
			continue
		}
		seen[id] = true
		if err := s.voiceCharacter(*job.ProjectID, id); err != nil {
			return err
		}
		doc, err := s.database.GetVoiceDocument(*job.ProjectID, "profile", id)
		if err != nil {
			name := id
			if character, e := s.database.GetCharacter(id); e == nil && character != nil {
				name = character.Name
			}
			return fmt.Errorf("set up and save a voice for %s before rendering: /voices?character_id=%s (choose Fish Audio or fal and a saved voice ID)", name, id)
		}
		var profile VoiceProfile
		if err = json.Unmarshal(doc.Data, &profile); err != nil || !profile.Approved {
			return fmt.Errorf("speaker %s voice is not approved; review it in /voices", id)
		}
		if profile.Mode == "native_reference" {
			return fmt.Errorf("speaker %s uses native reference audio: ordinary image-to-video cannot preserve its sample; use Prepare native reference video in /voices or explicitly choose an approved saved voice", id)
		}
		if (profile.Mode != "" && profile.Mode != "saved") || strings.TrimSpace(profile.VoiceID) == "" {
			return fmt.Errorf("speaker %s has no supported saved voice; create or bind it in /voices", id)
		}
		if profile.SampleAssetID != "" {
			_, _, hash, err := s.voiceAsset(*job.ProjectID, profile.SampleAssetID)
			if err != nil || hash != profile.SampleHash {
				return fmt.Errorf("speaker %s approved sample changed; review its profile in /voices", id)
			}
			bindings = append(bindings, db.VoiceBinding{Kind: "asset", ID: profile.SampleAssetID, Hash: hash})
		}
		bindings = append(bindings, db.VoiceBinding{Kind: "profile", ID: id, Revision: doc.Revision})
		speakers = append(speakers, map[string]any{"character_id": id, "voice_id": profile.VoiceID, "provider": profile.Provider, "model": profile.Model, "profile_revision": doc.Revision})
	}
	if len(speakers) == 0 {
		payload["generate_audio"] = false
		payload["voice_policy"] = map[string]any{"mode": "silent_visual_pass", "project_id": *job.ProjectID, "speech_stage": "/voices"}
		payload["prompt"] = stringPrompt(payload["prompt"]) + "\nSilent visual pass only. Add approved speaker audio in the explicit Voices workflow."
		job.RequestJSON = voiceJSON(payload)
		return nil
	}
	policy := map[string]any{"mode": "saved_voice_visual_pass", "project_id": *job.ProjectID, "speakers": speakers, "source_bindings": bindings, "speech_stage": "/voices", "visible_speaker_lip_sync_required": true}
	payload["generate_audio"] = false
	payload["voice_policy"] = policy
	payload["prompt"] = stringPrompt(payload["prompt"]) + "\nAuthoritative voice policy: generate a SILENT visual pass with NO dialogue, narration or generic substitute voice. Approved speaker identities are reserved for explicit saved-voice speech and visible-speaker lip sync stages in /voices. " + string(voiceJSON(policy))
	job.RequestJSON = voiceJSON(payload)
	return nil
}
func stringPrompt(value any) string { prompt, _ := value.(string); return prompt }
