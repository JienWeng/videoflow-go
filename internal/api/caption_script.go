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
	// ShotID is the shot the line is spoken in. Lines saved before lines had shots, and
	// lines nobody placed, have none: the speech budget places them by time
	// (speech_budget.go) without saving that guess.
	ShotID string `json:"shot_id,omitempty"`
	// StartHint is when the line starts, in seconds from the start of its shot (from the
	// start of the scene when it has no shot); nil when it simply follows the line before.
	StartHint *float64 `json:"start_hint,omitempty"`
}

// sceneSpokenLines are the lines a scene's script has its characters or narrator speak.
func sceneSpokenLines(scene *models.Scene) []SpokenLine {
	var script struct {
		Lines []SpokenLine `json:"spoken_lines"`
	}
	_ = json.Unmarshal(scene.SceneJSON, &script) // unreadable scene metadata has no spoken lines
	return script.Lines
}
