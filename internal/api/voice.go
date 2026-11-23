package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"math"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// RegisterVoiceCaptionRoutes is called once during route setup. These routes keep
// one saved voice per character and the voice catalogue; none of them makes speech.
func (s *Server) RegisterVoiceCaptionRoutes() {
	s.router.Get("/voice/state", s.handleVoiceState)
	s.router.Get("/voice/catalog/{provider}/models", s.handleSpeechModels)
	s.router.Get("/voice/catalog/{provider}/voices", s.handleSpeechVoices)
	s.router.Get("/voice/preferences", s.handleVoicePreferences)
	s.router.Put("/voice/preferences", s.handleVoicePreferences)
	s.router.Put("/voice/profiles/{character}", s.handleVoiceProfile)
	s.router.Post("/voice/samples/{character}", s.handleVoiceSample)
	s.router.Delete("/voice/samples/{character}", s.handleDeleteVoiceSample)
}

// VoiceProfile is the saved voice of one character. Profiles saved by earlier
// versions carry extra fields; they are ignored on load.
type VoiceProfile struct {
	CharacterID string `json:"character_id"`
	Provider    string `json:"provider"`
	Model       string `json:"model,omitempty"`
	VoiceID     string `json:"voice_id"`
	Approved    bool   `json:"approved"`
}

func voiceDecode(w http.ResponseWriter, r *http.Request, v any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		http.Error(w, "invalid voice request: "+err.Error(), 400)
		return false
	}
	return true
}
func (s *Server) voiceProject(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return "", false
	}
	if expected := r.URL.Query().Get("project_id"); expected != "" && expected != id {
		http.Error(w, "active project changed; reload voice workspace", 409)
		return "", false
	}
	return id, true
}
func voiceJSON(v any) json.RawMessage { raw, _ := json.Marshal(v); return raw }
func voiceHash(b []byte) string       { hash := sha256.Sum256(b); return hex.EncodeToString(hash[:]) }
func (s *Server) voiceCharacter(project, id string) error {
	c, err := s.database.GetCharacter(id)
	if err != nil || c.ProjectID == nil || *c.ProjectID != project {
		return fmt.Errorf("speaker character does not belong to this project")
	}
	return nil
}

// speechProvider reports whether a service speaks lines: Fish Audio, ElevenLabs, or
// Google Gemini's prebuilt voices on the Gemini key.
func speechProvider(name string) bool {
	return name == "fish" || name == "elevenlabs" || name == "gemini"
}

// primaryVoiceProvider is the provider the voice picker opens on. "fal" was a
// choice in earlier versions and now reads as Fish Audio.
func (s *Server) primaryVoiceProvider() string {
	if provider := s.routes().Setting("voice_primary_provider", "fish"); speechProvider(provider) {
		return provider
	}
	return "fish"
}

// voiceRequireMedia returns the length of a local audio or video file, or an error
// when the file is not that kind of media.
func voiceRequireMedia(ctx context.Context, path, kind string) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file,pipe", "-format_whitelist", "wav,mp3,flac,mov,matroska,webm", "-show_entries", "format=duration:stream=codec_type", "-of", "json", path).Output()
	if err != nil || len(raw) > 1<<20 {
		return 0, fmt.Errorf("FFprobe could not inspect media; install FFmpeg and provide valid audio/video")
	}
	var info struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Kind string `json:"codec_type"`
		} `json:"streams"`
	}
	if err = json.Unmarshal(raw, &info); err != nil {
		return 0, fmt.Errorf("invalid media metadata")
	}
	seconds, err := strconv.ParseFloat(info.Format.Duration, 64)
	if err != nil || seconds <= 0 || math.IsInf(seconds, 0) || math.IsNaN(seconds) {
		return 0, fmt.Errorf("invalid media duration")
	}
	audio, video := false, false
	for _, stream := range info.Streams {
		audio = audio || stream.Kind == "audio"
		video = video || stream.Kind == "video"
	}
	if (kind == "video" && !video) || (kind == "audio" && (!audio || video)) {
		return 0, fmt.Errorf("expected %s media stream", kind)
	}
	return seconds, nil
}
func (s *Server) handleVoiceState(w http.ResponseWriter, r *http.Request) {
	project, ok := s.voiceProject(w, r)
	if !ok {
		return
	}
	profiles, err := s.database.ListVoiceDocuments(project, "profile")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]any{"project_id": project, "profiles": profiles, "samples": s.voiceSamples(project), "primary_provider": s.primaryVoiceProvider()})
}
func (s *Server) handleVoiceProfile(w http.ResponseWriter, r *http.Request) {
	project, ok := s.voiceProject(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "character")
	if err := s.voiceCharacter(project, id); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var body struct {
		Revision int          `json:"revision"`
		Data     VoiceProfile `json:"data"`
	}
	if !voiceDecode(w, r, &body) {
		return
	}
	p := &body.Data
	p.CharacterID = id
	p.VoiceID = strings.TrimSpace(p.VoiceID)
	if p.Provider == "" {
		p.Provider = s.primaryVoiceProvider()
	}
	if !speechProvider(p.Provider) {
		http.Error(w, "voice provider must be fish, elevenlabs or gemini", 400)
		return
	}
	if p.Model == "" {
		p.Model = "auto"
	}
	if _, err := s.resolveSpeechModel(r.Context(), p.Provider, p.Model); err != nil {
		http.Error(w, "unsupported or unavailable speech model; refresh the official catalog", 400)
		return
	}
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	saved, err := s.database.SaveVoiceDocument(project, "profile", id, body.Revision, voiceJSON(p))
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	writeJSON(w, 200, saved)
}
