package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
)

const (
	maxSpokenLines        = 100
	maxSpokenLineRunes    = 1000
	maxSpokenLanguageSize = 64
	maxSpokenLineIDSize   = 64
)

// handleSceneSpokenLines replaces what a scene's characters and narrator say. The lines
// are part of the scene's inputs, so clips and storyboards made from the old lines no
// longer match the scene and are made again on the next render.
func (s *Server) handleSceneSpokenLines(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	scene, sceneErr := s.database.GetScene(chi.URLParam(r, "id"))
	if err != nil || sceneErr != nil || scene.ProjectID == nil || *scene.ProjectID != project {
		http.Error(w, "scene not found in active project", http.StatusNotFound)
		return
	}
	var body struct {
		Lines             []SpokenLine `json:"lines"`
		ExpectedUpdatedAt time.Time    `json:"expected_updated_at"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512*1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || body.Lines == nil {
		http.Error(w, "invalid spoken script", http.StatusBadRequest)
		return
	}
	if !scene.UpdatedAt.Equal(body.ExpectedUpdatedAt) {
		http.Error(w, "scene changed; reload before editing its spoken script", http.StatusConflict)
		return
	}
	lines, err := s.validSpokenLines(project, body.Lines)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var snapshot map[string]json.RawMessage
	if err = json.Unmarshal(scene.SceneJSON, &snapshot); err != nil && len(scene.SceneJSON) > 0 {
		http.Error(w, "scene metadata is invalid", http.StatusConflict)
		return
	}
	if snapshot == nil {
		snapshot = make(map[string]json.RawMessage)
	}
	old, _ := json.Marshal(scene)
	if _, err = s.database.CreateRevision("scene", scene.ID, old); err != nil {
		http.Error(w, "cannot save scene revision", http.StatusInternalServerError)
		return
	}
	snapshot["spoken_lines"], _ = json.Marshal(lines)
	scene.SceneJSON, _ = json.Marshal(snapshot)
	if err = s.database.UpdateScene(scene); err != nil {
		http.Error(w, "could not save spoken script; reload and try again", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, scene)
}

// validSpokenLines trims each line and checks it can be spoken: text and language are
// set, ids are unique (a missing or repeated id gets a new one, so kept lines keep theirs), and the speaker is the narrator or
// one of the project's characters. Only a character can be seen saying a line.
func (s *Server) validSpokenLines(project string, lines []SpokenLine) ([]SpokenLine, error) {
	if len(lines) > maxSpokenLines {
		return nil, fmt.Errorf("a scene can have at most %d spoken lines", maxSpokenLines)
	}
	taken, seen := map[string]bool{}, map[string]bool{}
	for _, line := range lines {
		taken[strings.TrimSpace(line.ID)] = true
	}
	speakers := map[string]bool{}
	out := make([]SpokenLine, 0, len(lines))
	for i, line := range lines {
		n := i + 1
		line.ID = strings.TrimSpace(line.ID)
		line.CharacterID = strings.TrimSpace(line.CharacterID)
		line.Language = strings.TrimSpace(line.Language)
		line.Text = strings.TrimSpace(line.Text)
		if line.Text == "" {
			return nil, fmt.Errorf("line %d has no text; delete it or write what is said", n)
		}
		if utf8.RuneCountInString(line.Text) > maxSpokenLineRunes {
			return nil, fmt.Errorf("line %d is too long (at most %d characters)", n, maxSpokenLineRunes)
		}
		if line.Language == "" || len(line.Language) > maxSpokenLanguageSize {
			return nil, fmt.Errorf("line %d needs a language", n)
		}
		if len(line.ID) > maxSpokenLineIDSize {
			return nil, fmt.Errorf("line %d has an invalid id", n)
		}
		if seen[line.ID] {
			line.ID = "" // a repeated id stays with its first line
		}
		for k := n; line.ID == ""; k++ {
			if id := fmt.Sprintf("line_%d", k); !taken[id] {
				line.ID = id
			}
		}
		taken[line.ID], seen[line.ID] = true, true
		if line.CharacterID == "" {
			line.Visible = false // the narrator is heard, never seen speaking
		} else if !speakers[line.CharacterID] {
			c, err := s.database.GetCharacter(line.CharacterID)
			if err != nil || c.ProjectID == nil || *c.ProjectID != project {
				return nil, fmt.Errorf("line %d is spoken by a character that is not in this project", n)
			}
			speakers[line.CharacterID] = true
		}
		out = append(out, line)
	}
	return out, nil
}
