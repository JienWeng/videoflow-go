package api

type SpokenLine struct {
	ID          string `json:"id"`
	CharacterID string `json:"character_id"`
	Language    string `json:"language"`
	Text        string `json:"text"`
	Visible     bool   `json:"visible"`
}
