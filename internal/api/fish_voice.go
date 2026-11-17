package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

// Called with recoveryMu held, after fingerprint and source checks. Claim fences
// synchronous TTS just like queued voice submissions; failures never auto-retry.
func (s *Server) runSavedSpeech(w http.ResponseWriter, r *http.Request, task *db.VoiceTask) {
	provider := strings.SplitN(task.Model, "/", 2)[0]
	key, endpoint, _, err := s.routes().Credentials(provider)
	if err != nil || endpoint != task.Endpoint {
		http.Error(w, "Speech provider connection missing or changed; restore it before running", 400)
		return
	}
	if err = s.database.ClaimVoiceTask(task.ProjectID, task.ID); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	var input map[string]any
	json.Unmarshal(task.Input, &input)
	var audio []byte
	if provider == "elevenlabs" {
		audio, err = providers.ElevenSpeech(r.Context(), key, endpoint, strings.TrimPrefix(task.Model, "elevenlabs/"), input)
	} else {
		audio, err = providers.NewFishClientAt(key, endpoint).Speech(r.Context(), strings.TrimPrefix(task.Model, "fish/"), input)
	}
	if err == nil {
		err = s.importSavedSpeech(r, task, audio)
	}
	if err != nil {
		task.Status = "uncertain"
		task.Error = provider + " speech did not finish locally. Check provider activity; this task will not resubmit automatically. " + err.Error()
	}
	if err = s.database.UpdateVoiceTask(task); err != nil {
		http.Error(w, "speech submission fenced; local receipt could not be saved", 503)
		return
	}
	writeJSON(w, 200, task)
}
func (s *Server) importSavedSpeech(r *http.Request, task *db.VoiceTask, audio []byte) error {
	id := "voice_" + task.ID
	dir := filepath.Join(s.cfg.StorageRoot, "assets")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".fish-*.mp3")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(audio); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	duration, err := voiceRequireMedia(r.Context(), f.Name(), "audio")
	if err != nil {
		return fmt.Errorf("invalid provider speech: %w", err)
	}
	if err = s.checkVoiceBindings(task); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, id+".mp3")); err != nil {
		return err
	}
	public := "storage/assets/" + id + ".mp3"
	script := voiceBinding(task, "script")
	utterance := voiceBinding(task, "utterance")
	character := voiceBinding(task, "profile").ID
	meta := map[string]any{"provider": strings.SplitN(task.Model, "/", 2)[0], "model": task.Model, "voice_task_id": task.ID, "source_bindings": task.Bindings, "script_id": script.ID, "script_revision": script.Revision, "line_id": utterance.ID, "character_id": character, "duration_seconds": duration}
	asset := &models.Asset{ID: id, ProjectID: &task.ProjectID, CharacterID: &character, Type: "audio", Name: strings.SplitN(task.Model, "/", 2)[0] + " character speech", FilePath: public, MetadataJSON: voiceJSON(meta)}
	if err = s.database.CreateAsset(asset); err != nil {
		return err
	}
	task.Status = "completed"
	task.Error = ""
	task.Result = voiceJSON(map[string]any{"asset_id": id, "local_path": public, "provider": strings.SplitN(task.Model, "/", 2)[0]})
	return nil
}
