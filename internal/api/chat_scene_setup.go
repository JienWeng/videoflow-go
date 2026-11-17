package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strings"
	"time"
	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
)

type sceneSpeakerSetup struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Language string       `json:"language"`
	Revision int          `json:"revision"`
	Profile  VoiceProfile `json:"profile"`
	Ready    bool         `json:"ready"`
	Issue    string       `json:"issue,omitempty"`
}

func (s *Server) sceneSetup(ctx context.Context, scene *models.Scene) (map[string]any, error) {
	project := *scene.ProjectID
	brief := sceneBrief(scene)
	var cast []string
	if err := json.Unmarshal(scene.CharacterIDsJSON, &cast); err != nil {
		return nil, err
	}
	cast = append(cast, brief.CharacterIDs...)
	var data struct {
		Lines []SpokenLine `json:"spoken_lines"`
	}
	if err := json.Unmarshal(scene.SceneJSON, &data); err != nil {
		return nil, err
	}
	language := brief.Language
	if language == "" {
		language = "en"
	}
	languages := map[string]string{}
	mode := brief.ConversationMode
	if !s.cfg.UseLocalText || mode == "silent" {
		cast = nil
	} else if len(data.Lines) > 0 {
		cast = nil
		for _, l := range data.Lines {
			if strings.TrimSpace(l.Text) != "" {
				if l.CharacterID == "" {
					return nil, fmt.Errorf("a spoken line has no speaker; assign its character before rendering")
				}
				cast = append(cast, l.CharacterID)
				if l.Language != "" {
					language = l.Language
					languages[l.CharacterID] = l.Language
				}
			}
		}
	}
	speakers := []sceneSpeakerSetup{}
	seen := map[string]bool{}
	allReady := true
	for _, id := range cast {
		if seen[id] {
			continue
		}
		seen[id] = true
		c, err := s.database.GetCharacter(id)
		if err != nil || c.ProjectID == nil || *c.ProjectID != project {
			return nil, fmt.Errorf("scene speaker is unavailable in this project")
		}
		row := sceneSpeakerSetup{ID: id, Name: c.Name, Language: languages[id]}
		if row.Language == "" {
			row.Language = language
		}
		row.Language = setupLanguage(row.Language)
		doc, e := s.database.GetVoiceDocument(project, "profile", id)
		if e != nil {
			row.Issue = "Choose a saved voice"
		} else {
			row.Revision = doc.Revision
			if json.Unmarshal(doc.Data, &row.Profile) != nil {
				row.Issue = "Voice profile needs replacement"
			} else if !row.Profile.Approved {
				row.Issue = "Voice selection has not been saved as approved"
			} else if row.Profile.Mode == "native_reference" {
				row.Issue = "Native sample mode needs a separate native-reference video; choose a saved voice for this scene render"
			} else if (row.Profile.Mode != "" && row.Profile.Mode != "saved") || strings.TrimSpace(row.Profile.VoiceID) == "" {
				row.Issue = "Choose a saved voice"
			} else if row.Profile.SampleAssetID != "" {
				_, _, hash, err := s.voiceAsset(project, row.Profile.SampleAssetID)
				if err != nil || hash != row.Profile.SampleHash {
					row.Issue = "Saved voice sample changed; choose and save a voice again"
				}
			}
		}
		row.Ready = row.Issue == ""
		if !row.Ready {
			allReady = false
		}
		speakers = append(speakers, row)
	}
	language = setupLanguage(language)
	refs, err := s.sceneReferenceState(ctx, scene)
	if err != nil {
		return nil, err
	}
	shots, err := s.database.ListShots(scene.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"project_id": project, "scene_id": scene.ID, "title": scene.Title, "updated_at": scene.UpdatedAt, "duration": scene.Duration, "aspect_ratio": scene.AspectRatio, "language": language, "speakers": speakers, "voices_ready": allReady, "references_ready": refs["references_ready"], "storyboard_ready": refs["storyboard_ready"], "missing_character_ids": refs["missing_character_ids"], "missing_asset_ids": refs["missing_asset_ids"], "has_shots": len(shots) > 0, "primary_provider": s.routes().Setting("voice_primary_provider", "fal")}, nil
}
func (s *Server) setupScene(w http.ResponseWriter, r *http.Request) (*models.Scene, bool) {
	project, ok := s.voiceProject(w, r)
	if !ok {
		return nil, false
	}
	scene, err := s.database.GetScene(chi.URLParam(r, "scene"))
	if err != nil || scene.ProjectID == nil || *scene.ProjectID != project {
		http.Error(w, "scene is not in the active project", 404)
		return nil, false
	}
	return scene, true
}
func (s *Server) handleSceneSetup(w http.ResponseWriter, r *http.Request) {
	scene, ok := s.setupScene(w, r)
	if !ok {
		return
	}
	state, err := s.sceneSetup(r.Context(), scene)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	writeJSON(w, 200, state)
}
func (s *Server) handleSaveSceneVoices(w http.ResponseWriter, r *http.Request) {
	scene, ok := s.setupScene(w, r)
	if !ok {
		return
	}
	var body struct {
		Expected  time.Time `json:"expected_updated_at"`
		Confirmed bool      `json:"confirmed"`
		Profiles  []struct {
			ID       string `json:"character_id"`
			Revision int    `json:"revision"`
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Voice    string `json:"voice_id"`
		} `json:"profiles"`
	}
	if !voiceDecode(w, r, &body) {
		return
	}
	if !body.Confirmed || body.Expected.IsZero() || len(body.Profiles) == 0 || len(body.Profiles) > 100 {
		http.Error(w, "confirm the displayed scene voices before saving", 400)
		return
	}
	state, err := s.sceneSetup(r.Context(), scene)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	allowed := map[string]bool{}
	for _, row := range state["speakers"].([]sceneSpeakerSetup) {
		allowed[row.ID] = !row.Ready
	}
	docs := []db.VoiceDocument{}
	seen := map[string]bool{}
	for _, p := range body.Profiles {
		if !allowed[p.ID] || seen[p.ID] || strings.TrimSpace(p.Voice) == "" || len(p.Voice) > 256 {
			http.Error(w, "choose one saved voice for each scene speaker", 400)
			return
		}
		seen[p.ID] = true
		if p.Provider != "fal" && p.Provider != "fish" && p.Provider != "elevenlabs" {
			http.Error(w, "choose a supported voice provider", 400)
			return
		}
		if _, _, _, err = s.routes().Credentials(p.Provider); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if p.Provider != "fal" {
			if _, err = s.resolveSpeechModel(r.Context(), p.Provider, p.Model); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
		}
		profile := VoiceProfile{CharacterID: p.ID, Provider: p.Provider, Model: p.Model, Mode: "saved", VoiceID: strings.TrimSpace(p.Voice), Approved: true}
		docs = append(docs, db.VoiceDocument{ID: p.ID, Revision: p.Revision, Data: voiceJSON(profile)})
	}
	for id, needed := range allowed {
		if needed && !seen[id] {
			http.Error(w, "choose voices for all missing speakers before saving", 400)
			return
		}
	}
	s.recoveryMu.Lock()
	err = s.database.SaveSceneVoices(*scene.ProjectID, scene.ID, body.Expected, docs)
	s.recoveryMu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	scene, err = s.database.GetScene(scene.ID)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	state, err = s.sceneSetup(r.Context(), scene)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	writeJSON(w, 200, state)
}

func setupLanguage(language string) string {
	aliases := map[string]string{"english": "en", "spanish": "es", "french": "fr", "german": "de", "chinese": "zh", "japanese": "ja", "korean": "ko", "portuguese": "pt"}
	if code := aliases[strings.ToLower(strings.TrimSpace(language))]; code != "" {
		return code
	}
	return language
}
