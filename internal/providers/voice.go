package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Curated schemas checked against official fal API documentation 2026-11-11.
const (
	FalVoiceCloneModel  = "fal-ai/minimax/voice-clone"
	FalSpeechModel      = "fal-ai/minimax/speech-02-hd"
	FalAlignmentModel   = "fal-ai/elevenlabs/forced-alignment"
	FalNativeVoiceModel = "bytedance/seedance-2.0/fast/reference-to-video"
	FalLipSyncModel     = "fal-ai/sync-lipsync/v2/pro"
)

func ValidateVoiceInput(model string, p map[string]any) error {
	required := func(key string) bool { return strings.TrimSpace(stringValue(p[key])) != "" }
	switch model {
	case FalVoiceCloneModel:
		if !required("audio_url") {
			return fmt.Errorf("voice creation requires approved audio sample")
		}
	case FalSpeechModel:
		setting, _ := p["voice_setting"].(map[string]any)
		if !required("text") || utf8.RuneCountInString(stringValue(p["text"])) > 5000 || strings.TrimSpace(stringValue(setting["voice_id"])) == "" {
			return fmt.Errorf("speech requires text (1–5000 characters) and an explicit saved voice ID")
		}
	case FalAlignmentModel:
		if !required("audio_url") || !required("text") {
			return fmt.Errorf("alignment requires final audio and exact approved script")
		}
	case FalLipSyncModel:
		if !required("audio_url") || !required("video_url") {
			return fmt.Errorf("lip sync requires final audio and video")
		}
	case FalNativeVoiceModel:
		// This endpoint is reference mode, never strict first/last frame mode.
		if !required("prompt") {
			return fmt.Errorf("native reference video requires a prompt")
		}
		if _, ok := p["image_url"]; ok {
			return fmt.Errorf("strict first/last frames cannot be mixed with native audio references")
		}
		if _, ok := p["end_image_url"]; ok {
			return fmt.Errorf("strict first/last frames cannot be mixed with native audio references")
		}
		count := func(key string) int {
			b, _ := json.Marshal(p[key])
			var v []string
			json.Unmarshal(b, &v)
			return len(v)
		}
		a, i, v := count("audio_urls"), count("image_urls"), count("video_urls")
		if a < 1 || a > 3 || i+v < 1 || i > 9 || v > 3 || a+i+v > 12 {
			return fmt.Errorf("native reference mode needs 1–3 audio references, 1–9 images or 1–3 videos, at most 12 total files")
		}
	default:
		return fmt.Errorf("unsupported voice endpoint %q", model)
	}
	return nil
}
func (c *FalClient) SubmitVoice(ctx context.Context, model string, p map[string]any) (string, error) {
	if err := ValidateVoiceInput(model, p); err != nil {
		return "", err
	}
	return c.submit(ctx, model, p)
}

// VoiceResult performs retrieval only. It never submits or retries generation.
func (c *FalClient) VoiceResult(ctx context.Context, model, id string) (string, json.RawMessage, error) {
	switch model {
	case FalVoiceCloneModel, FalSpeechModel, FalAlignmentModel, FalNativeVoiceModel, FalLipSyncModel:
	default:
		return "", nil, fmt.Errorf("unsupported voice endpoint")
	}
	return c.result(ctx, model, id)
}

// DownloadVoiceResult uses the existing public-media boundary and SSRF checks.
func (c *FalClient) DownloadVoiceResult(ctx context.Context, url string, limit int64) ([]byte, error) {
	return publicMedia(ctx, url, limit, c.base)
}
