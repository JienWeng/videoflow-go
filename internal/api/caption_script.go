package api

import (
	"encoding/json"
	"videoflow-go/internal/models"
)

type SpokenLine struct {
	ID          string `json:"id"`
	CharacterID string `json:"character_id"`
	Language    string `json:"language"`
	Text        string `json:"text"`
	Visible     bool   `json:"visible"`
}

// ScriptCaptionPayload replaces the caption lines and style of an output and keeps
// whatever else was stored beside them.
func (s *Server) ScriptCaptionPayload(output *models.RenderOutput, segments []models.CaptionSegment, style string) json.RawMessage {
	var payload map[string]any
	if json.Unmarshal(output.CaptionsJSON, &payload) != nil || payload == nil {
		payload = map[string]any{}
	}
	payload["segments"] = segments
	payload["style"] = style
	return voiceJSON(payload)
}
