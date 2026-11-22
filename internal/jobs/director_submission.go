package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"videoflow-go/internal/models"
)

func (w *WorkerPool) SetSubmissionValidator(validate func(context.Context, *models.RenderJob) error) {
	w.validationMu.Lock()
	defer w.validationMu.Unlock()
	w.submissionValidator = validate
}
func (w *WorkerPool) validateSubmission(ctx context.Context, job *models.RenderJob) error {
	var raw map[string]json.RawMessage
	if json.Unmarshal(job.RequestJSON, &raw) != nil {
		return fmt.Errorf("invalid render request")
	}
	if raw["director_context"] == nil && raw["voice_policy"] == nil {
		return nil
	}
	w.validationMu.RLock()
	validate := w.submissionValidator
	w.validationMu.RUnlock()
	if validate == nil {
		return fmt.Errorf("frozen render source validation unavailable; request was not submitted")
	}
	return validate(ctx, job)
}
func providerVideoPayload(payload map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range payload {
		switch key {
		case "director_context", "director_keyframe", "voice_policy", "request_fingerprint", "audio_mode", "music_prompt", "render_unit", "render_plan_id", "scene_input_revision", "video_input", "reference_pictures":
			continue
		}
		out[key] = value
	}
	return out
}
