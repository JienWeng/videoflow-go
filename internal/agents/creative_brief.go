package agents

import "context"

// CreativeBrief is a proposal, not authorization to start media generation.
type CreativeBrief struct {
	StorytellingID   string `json:"storytelling_id,omitempty"`
	Instruction      string `json:"instruction,omitempty"`
	StyleFingerprint string `json:"style_fingerprint,omitempty"`
	Idea             string `json:"idea"`
	PresetID         string `json:"style_preset_id"`
	Tone             string `json:"tone"`
	Audience         string `json:"audience"`
	Platform         string `json:"platform"`
	AspectRatio      string `json:"aspect_ratio"`
	Resolution       string `json:"resolution"`
	Language         string `json:"language"`
	ConversationMode string `json:"conversation_mode"`
	// Music is a short direction for background music made with the video; empty means none.
	Music          string `json:"music"`
	TargetDuration int    `json:"target_duration"`
	SceneCount     int    `json:"scene_count"`
}

type creativeDirectionKey struct{}

func WithCreativeDirection(ctx context.Context, direction string) context.Context {
	return context.WithValue(ctx, creativeDirectionKey{}, direction)
}
