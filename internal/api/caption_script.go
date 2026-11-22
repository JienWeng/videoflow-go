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

// sceneSpokenLines are the lines a scene's script has its characters or narrator speak.
func sceneSpokenLines(scene *models.Scene) []SpokenLine {
	var script struct {
		Lines []SpokenLine `json:"spoken_lines"`
	}
	_ = json.Unmarshal(scene.SceneJSON, &script) // unreadable scene metadata has no spoken lines
	return script.Lines
}
