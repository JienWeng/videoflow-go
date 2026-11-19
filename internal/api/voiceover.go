package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

// Voice-over from the script: the same shot-level timing as captions-from-script,
// spoken with each character's approved saved voice (or a narrator voice chosen in
// the editor). Speech is cached by voice+text, so pressing the button again never
// pays twice for a line that has not changed.

type voiceChoice struct {
	Provider string `json:"provider"`
	VoiceID  string `json:"voice_id"`
	Model    string `json:"model,omitempty"`
}

func (v voiceChoice) usable() bool {
	return (v.Provider == "fish" || v.Provider == "elevenlabs") && providers.SafeSpeechID(v.VoiceID)
}

type voiceoverItem struct {
	AssetID  string  `json:"asset_id"`
	Start    float64 `json:"start"`
	Duration float64 `json:"duration"`
	Text     string  `json:"text"`
	Speaker  string  `json:"speaker"`
	Cached   bool    `json:"cached"`
}

// voiceoverSpeech is the one paid call; tests replace it.
var voiceoverSpeech = func(ctx context.Context, provider, key, endpoint, model, voice, text string) ([]byte, error) {
	input := map[string]any{"text": text, "reference_id": voice, "format": "mp3"}
	if provider == "elevenlabs" {
		return providers.ElevenSpeech(ctx, key, endpoint, model, input)
	}
	return providers.NewFishClientAt(key, endpoint).Speech(ctx, model, input)
}

const voiceoverMaxLines = 60

func (s *Server) handleCompositionVoiceover(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	var body struct {
		Clips    []composition.Clip `json:"clips"`
		Narrator voiceChoice        `json:"narrator"`
		// Lines are typed by the user in the editor: spoken by the narrator at Start.
		Lines []struct {
			Text  string  `json:"text"`
			Start float64 `json:"start"`
		} `json:"lines"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body) != nil {
		http.Error(w, "invalid voice-over request", 400)
		return
	}
	cues := []composition.Cue{}
	skipped := []map[string]string{}
	for _, clip := range body.Clips {
		if !strings.HasPrefix(clip.AssetID, "output-") {
			continue // Images and uploaded media have no script.
		}
		c, err := s.scriptCuesForClip(project, clip)
		if err != nil {
			skipped = append(skipped, map[string]string{"reason": err.Error()})
			continue
		}
		cues = append(cues, c...)
	}
	if len(body.Lines) > 10 {
		http.Error(w, "type at most 10 lines at a time", 400)
		return
	}
	for i, line := range body.Lines {
		text := strings.TrimSpace(line.Text)
		if text == "" || utf8.RuneCountInString(text) > 500 || !(line.Start >= 0 && line.Start <= 600) {
			http.Error(w, "each typed line needs 1-500 characters and a start time within the edit", 400)
			return
		}
		cues = append(cues, composition.Cue{ID: fmt.Sprintf("typed-%d", i), Start: line.Start, Text: text})
	}
	sort.SliceStable(cues, func(i, j int) bool { return cues[i].Start < cues[j].Start })
	if len(cues) > voiceoverMaxLines {
		http.Error(w, fmt.Sprintf("this edit has %d script lines; voice at most %d at a time", len(cues), voiceoverMaxLines), 400)
		return
	}

	names := map[string]string{}
	profiles := map[string]voiceChoice{}
	voiceFor := func(characterID string) (voiceChoice, string) {
		if characterID == "" {
			return body.Narrator, "Narrator"
		}
		if _, seen := names[characterID]; !seen {
			names[characterID] = "Narrator"
			if c, err := s.database.GetCharacter(characterID); err == nil && c.ProjectID != nil && *c.ProjectID == project {
				names[characterID] = c.Name
			}
			if doc, err := s.database.GetVoiceDocument(project, "profile", characterID); err == nil {
				var p VoiceProfile
				if json.Unmarshal(doc.Data, &p) == nil && p.Approved {
					profiles[characterID] = voiceChoice{Provider: p.Provider, VoiceID: p.VoiceID, Model: p.Model}
				}
			}
		}
		if v := profiles[characterID]; v.usable() {
			return v, names[characterID]
		}
		return body.Narrator, names[characterID]
	}

	items := []voiceoverItem{}
	generated, reused := 0, 0
	for _, cue := range cues {
		voice, speaker := voiceFor(cue.CharacterID)
		if !voice.usable() {
			skipped = append(skipped, map[string]string{"reason": fmt.Sprintf("No voice for %s. Choose a narrator voice, or set up %s's voice in Voices.", speaker, speaker)})
			continue
		}
		if utf8.RuneCountInString(cue.Text) > 5000 {
			skipped = append(skipped, map[string]string{"reason": "A script line is longer than 5000 characters"})
			continue
		}
		item, cached, err := s.voiceoverLine(r.Context(), project, voice, cue, speaker)
		if err != nil {
			// Stop at the first provider failure: never keep spending on a broken connection.
			writeJSON(w, 502, map[string]any{"error": err.Error(), "items": items, "skipped": skipped, "generated": generated, "reused": reused})
			return
		}
		if cached {
			reused++
		} else {
			generated++
		}
		items = append(items, item)
	}
	writeJSON(w, 200, map[string]any{"items": items, "skipped": skipped, "generated": generated, "reused": reused})
}

func (s *Server) voiceoverLine(ctx context.Context, project string, voice voiceChoice, cue composition.Cue, speaker string) (voiceoverItem, bool, error) {
	model, err := s.resolveSpeechModel(ctx, voice.Provider, voice.Model, cue.Language)
	if err != nil {
		return voiceoverItem{}, false, err
	}
	id := "voiceover_" + compositionHash([]byte(strings.Join([]string{project, voice.Provider, model, voice.VoiceID, cue.Text}, "\x00")))[:20]
	public := "storage/assets/" + id + ".mp3"
	item := voiceoverItem{AssetID: id, Start: cue.Start, Text: cue.Text, Speaker: speaker}
	if a, err := s.database.GetAsset(id); err == nil && a.ProjectID != nil && *a.ProjectID == project {
		if full, err := s.storagePath(a.FilePath); err == nil {
			if d, err := voiceRequireMedia(ctx, full, "audio"); err == nil {
				item.Duration, item.Cached = round3(d), true
				return item, true, nil
			}
		}
	}
	key, endpoint, _, err := s.routes().Credentials(voice.Provider)
	if err != nil {
		return item, false, fmt.Errorf("connect %s in Settings first: %w", voice.Provider, err)
	}
	audio, err := voiceoverSpeech(ctx, voice.Provider, key, endpoint, model, voice.VoiceID, cue.Text)
	if err != nil {
		return item, false, err
	}
	dir := filepath.Join(s.cfg.StorageRoot, "assets")
	if err = os.MkdirAll(dir, 0755); err != nil {
		return item, false, err
	}
	f, err := os.CreateTemp(dir, ".voiceover-*.mp3")
	if err != nil {
		return item, false, err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(audio); err != nil {
		f.Close()
		return item, false, err
	}
	if err = f.Close(); err != nil {
		return item, false, err
	}
	duration, err := voiceRequireMedia(ctx, f.Name(), "audio")
	if err != nil {
		return item, false, fmt.Errorf("the speech provider returned unusable audio: %w", err)
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, id+".mp3")); err != nil {
		return item, false, err
	}
	label := cue.Text
	if utf8.RuneCountInString(label) > 40 {
		label = string([]rune(label)[:40]) + "…"
	}
	meta, _ := json.Marshal(map[string]any{"voiceover": true, "text": cue.Text, "character_id": cue.CharacterID, "provider": voice.Provider, "model": model, "voice_id": voice.VoiceID})
	a := &models.Asset{ID: id, ProjectID: &project, Type: "voiceover", Name: "Voice · " + speaker + ": " + label + ".mp3", FilePath: public, TagsJSON: []byte("[]"), MetadataJSON: meta}
	if _, err := s.database.GetAsset(id); err != nil {
		if err = s.database.CreateUploadedAsset(a); err != nil {
			return item, false, err
		}
	}
	item.Duration = round3(duration)
	return item, false, nil
}
