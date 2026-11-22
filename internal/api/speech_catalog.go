package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

type speechCatalog struct {
	Models  []providers.SpeechModel `json:"models"`
	Checked time.Time               `json:"checked_at"`
	Source  string                  `json:"source"`
	Stale   bool                    `json:"stale"`
	Warning string                  `json:"warning,omitempty"`
}

func (s *Server) speechModels(ctx context.Context, provider string, refresh bool) (speechCatalog, error) {
	cat := speechCatalog{}
	key, base, _, err := s.routes().Credentials(provider)
	if provider != "fish" && err != nil {
		return cat, err
	}
	cacheKey := "voice_catalog_" + provider + "_" + voiceHash([]byte(key + base))[:16]
	raw, _ := s.database.GetAppSetting(cacheKey)
	_ = json.Unmarshal([]byte(raw), &cat) // an unreadable cache is fetched again
	if !refresh && len(cat.Models) > 0 && time.Since(cat.Checked) < 24*time.Hour {
		return cat, nil
	}
	schema := s.cfg.FishSchemaURL
	if schema == "" {
		schema = "https://docs.fish.audio/api-reference/openapi.json"
	}
	models, err := providers.SpeechModels(ctx, provider, key, base, schema)
	if err != nil || len(models) == 0 {
		if err == nil {
			err = fmt.Errorf("provider returned no compatible speech models")
		}
		if len(cat.Models) > 0 {
			cat.Stale = true
			cat.Warning = err.Error()
			encoded, _ := json.Marshal(cat)
			_ = s.database.SetAppSetting(cacheKey, string(encoded))
			return cat, nil
		}
		return cat, err
	}
	cat = speechCatalog{Models: models, Checked: time.Now().UTC(), Source: base + "/v1/models"}
	if provider == "fish" {
		cat.Source = "https://docs.fish.audio/api-reference/openapi.json"
	}
	encoded, _ := json.Marshal(cat)
	if err = s.database.SetAppSetting(cacheKey, string(encoded)); err != nil {
		return cat, err
	}
	return cat, nil
}
func (s *Server) resolveSpeechModel(ctx context.Context, provider, model string, language ...string) (string, error) {
	if model == "" {
		model = "auto"
	}
	// Previously supported pinned Fish models remain usable during a catalog outage.
	if provider == "fish" && providers.FishModel(model) {
		return model, nil
	}
	cat, err := s.speechModels(ctx, provider, false)
	if err != nil {
		return "", err
	}
	compatible := func(m providers.SpeechModel) bool {
		if len(language) == 0 || len(m.Languages) == 0 {
			return true
		}
		lang := strings.ToLower(strings.SplitN(language[0], "-", 2)[0])
		aliases := map[string]string{"english": "en", "spanish": "es", "french": "fr", "german": "de", "chinese": "zh", "japanese": "ja", "korean": "ko", "portuguese": "pt"}
		if a := aliases[lang]; a != "" {
			lang = a
		}
		if len(lang) > 3 {
			return true
		}
		for _, l := range m.Languages {
			if strings.EqualFold(l, lang) {
				return true
			}
		}
		return false
	}
	if model == "auto" {
		for _, m := range cat.Models {
			if m.Default && compatible(m) {
				return m.ID, nil
			}
		}
		for _, m := range cat.Models {
			if compatible(m) {
				return m.ID, nil
			}
		}
	}
	for _, m := range cat.Models {
		if m.ID == model && compatible(m) {
			return model, nil
		}
	}
	return "", fmt.Errorf("speech model is not in the official provider catalog; refresh models or choose another model")
}
func (s *Server) handleSpeechModels(w http.ResponseWriter, r *http.Request) {
	p := chi.URLParam(r, "provider")
	if p != "fish" && p != "elevenlabs" {
		http.Error(w, "choose Fish Audio or ElevenLabs", 400)
		return
	}
	cat, err := s.speechModels(r.Context(), p, r.URL.Query().Get("refresh") == "true")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, 200, cat)
}
func voiceMatch(voices []providers.SpeechVoice, description string) (*providers.SpeechVoice, string) {
	if len(voices) == 0 {
		return nil, "No voices found for these filters"
	}
	stop := map[string]bool{"with": true, "this": true, "that": true, "character": true, "voice": true, "from": true, "have": true, "their": true, "the": true, "and": true, "for": true, "she": true, "his": true, "her": true}
	tokens := strings.FieldsFunc(strings.ToLower(description), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	words := map[string]bool{}
	for _, t := range tokens {
		if len(t) > 2 && !stop[t] {
			words[t] = true
		}
	}
	score := func(v providers.SpeechVoice) (int, []string) {
		labels, _ := json.Marshal(v.Labels)
		text := strings.ToLower(v.Name + " " + v.Description + " " + string(labels))
		parts := map[string]bool{}
		for _, t := range strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			parts[t] = true
		}
		matched := []string{}
		for t := range words {
			if parts[t] {
				matched = append(matched, t)
			}
		}
		sort.Strings(matched)
		return len(matched), matched
	}
	sort.SliceStable(voices, func(i, j int) bool {
		a, _ := score(voices[i])
		b, _ := score(voices[j])
		if a != b {
			return a > b
		}
		return voices[i].ID < voices[j].ID
	})
	_, matches := score(voices[0])
	reason := "No descriptive match found; selected a voice returned by your language/search filters. Audition it before saving."
	if len(matches) > 0 {
		reason = "Matched voice metadata: " + strings.Join(matches, ", ") + ". Audition it before saving."
	}
	return &voices[0], reason
}
func (s *Server) handleSpeechVoices(w http.ResponseWriter, r *http.Request) {
	project, ok := s.voiceProject(w, r)
	if !ok {
		return
	}
	p := chi.URLParam(r, "provider")
	if p != "fish" && p != "elevenlabs" {
		http.Error(w, "choose Fish Audio or ElevenLabs", 400)
		return
	}
	// Without a character (narrator voice in the editor) the list is unranked.
	var character *models.Character
	if id := r.URL.Query().Get("character_id"); id != "" {
		c, err := s.database.GetCharacter(id)
		if err != nil || c.ProjectID == nil || *c.ProjectID != project {
			http.Error(w, "character not in active project", 404)
			return
		}
		character = c
	}
	q := r.URL.Query()
	if len(q.Get("search")) > 160 || len(q.Get("language")) > 35 || len(q.Get("page")) > 512 {
		http.Error(w, "voice filter is too long", 400)
		return
	}
	key, base, _, err := s.routes().Credentials(p)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	list, err := providers.ListSpeechVoices(r.Context(), p, key, base, q.Get("search"), q.Get("language"), q.Get("page"))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if character == nil {
		writeJSON(w, 200, map[string]any{"voices": list.Voices, "next_page": list.Next, "checked_at": time.Now().UTC()})
		return
	}
	rules, _ := json.Marshal(character.VoiceRulesJSON)
	best, reason := voiceMatch(list.Voices, character.Description+" "+character.Personality+" "+string(rules))
	writeJSON(w, 200, map[string]any{"voices": list.Voices, "next_page": list.Next, "suggested": best, "reason": reason, "checked_at": time.Now().UTC()})
}
func (s *Server) handleVoicePreferences(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		writeJSON(w, 200, map[string]string{"primary_provider": s.primaryVoiceProvider()})
		return
	}
	var body struct {
		Primary string `json:"primary_provider"`
	}
	if !voiceDecode(w, r, &body) {
		return
	}
	if body.Primary != "fish" && body.Primary != "elevenlabs" {
		http.Error(w, "choose an available speech provider", 400)
		return
	}
	if _, _, _, err := s.routes().Credentials(body.Primary); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := s.database.SetAppSetting("voice_primary_provider", string(voiceJSON(body.Primary))); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, body)
}
