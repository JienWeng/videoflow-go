package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"videoflow-go/internal/models"
)

// A character's voice sample is a short recording of how the character sounds. Clips made
// from reference pictures are sent it too, so a video model that takes voice references
// gives the character the same voice in every clip instead of inventing one each time.
// A service takes a few seconds of audio per clip in total, so each sample is kept short.
const (
	voiceSampleType       = "voice_sample"
	voiceSampleSeconds    = 5
	voiceSampleMinSeconds = 2
)

func voiceSampleID(character string) string { return "voice_sample_" + character }

// voiceSample is the character's saved sample, or nil when there is none to send.
func (s *Server) voiceSample(project, character string) *models.Asset {
	asset, err := s.database.GetAsset(voiceSampleID(character))
	if err != nil || asset.Type != voiceSampleType || asset.ProjectID == nil || *asset.ProjectID != project {
		return nil
	}
	if full, err := s.storagePath(asset.FilePath); err != nil {
		return nil
	} else if _, err = os.Stat(full); err != nil {
		return nil
	}
	return asset
}

// voiceSamples lists a project's samples by character, for the Voices page.
func (s *Server) voiceSamples(project string) map[string]string {
	out := map[string]string{}
	assets, _ := s.database.ListAssets(project)
	for _, a := range assets {
		if a.Type == voiceSampleType && a.CharacterID != nil {
			out[*a.CharacterID] = a.FilePath
		}
	}
	return out
}

// handleVoiceSample saves a character's voice sample: from an uploaded recording, or,
// with no file, spoken by the voice already saved for the character.
func (s *Server) handleVoiceSample(w http.ResponseWriter, r *http.Request) {
	project, ok := s.voiceProject(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "character")
	character, err := s.database.GetCharacter(id)
	if err != nil || character.ProjectID == nil || *character.ProjectID != project {
		http.Error(w, "character not found in this project", 400)
		return
	}
	dir := filepath.Join(s.cfg.StorageRoot, "assets")
	if err = os.MkdirAll(dir, 0755); err != nil {
		http.Error(w, "failed to prepare storage", 500)
		return
	}
	source, err := os.CreateTemp(dir, ".voice-sample-*")
	if err != nil {
		http.Error(w, "failed to prepare storage", 500)
		return
	}
	defer os.Remove(source.Name())
	defer source.Close()

	origin := "upload"
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, 20<<20)
		file, _, e := r.FormFile("file")
		if e != nil {
			http.Error(w, "choose an audio file of at most 20 MB", 400)
			return
		}
		defer file.Close()
		if _, err = io.Copy(source, file); err != nil {
			http.Error(w, "failed to save the recording", 500)
			return
		}
	} else {
		origin = "voice"
		audio, e := s.speakVoiceSample(r.Context(), project, character)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		if _, err = source.Write(audio); err != nil {
			http.Error(w, "failed to save the recording", 500)
			return
		}
	}
	if err = source.Close(); err != nil {
		http.Error(w, "failed to save the recording", 500)
		return
	}
	seconds, err := voiceRequireMedia(r.Context(), source.Name(), "audio")
	if err != nil {
		http.Error(w, "this is not a recording the app can read; use an MP3, WAV, M4A or FLAC file", 400)
		return
	}
	if seconds < voiceSampleMinSeconds {
		http.Error(w, fmt.Sprintf("a voice sample needs at least %d seconds of speech", voiceSampleMinSeconds), 400)
		return
	}
	// One mono MP3 of the opening seconds: what every service that takes voice references reads.
	trimmed := source.Name() + ".mp3"
	defer os.Remove(trimmed)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if out, e := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-y", "-protocol_whitelist", "file,pipe", "-i", source.Name(), "-t", fmt.Sprint(voiceSampleSeconds), "-vn", "-ac", "1", "-ar", "44100", "-b:a", "96k", "-f", "mp3", trimmed).CombinedOutput(); e != nil {
		http.Error(w, "could not prepare the recording: "+strings.TrimSpace(string(out)), 400)
		return
	}
	assetID := voiceSampleID(id)
	if err = os.Rename(trimmed, filepath.Join(dir, assetID+".mp3")); err != nil {
		http.Error(w, "failed to save the recording", 500)
		return
	}
	meta, _ := json.Marshal(map[string]any{"voice_sample": true, "source": origin})
	asset := &models.Asset{ID: assetID, ProjectID: &project, Type: voiceSampleType, Name: character.Name + " voice sample.mp3", FilePath: "storage/assets/" + assetID + ".mp3", TagsJSON: []byte("[]"), MetadataJSON: meta, CharacterID: &id}
	if _, e := s.database.GetAsset(assetID); e != nil {
		err = s.database.CreateUploadedAsset(asset)
	} else {
		err = s.database.UpdateAsset(asset)
	}
	if err != nil {
		http.Error(w, "failed to save the voice sample", 500)
		return
	}
	sample := "uploaded recording"
	if origin == "voice" {
		sample = "spoken by the saved voice"
	}
	s.recordDecision(project, "voice_sample:"+id, sample)
	writeJSON(w, 200, map[string]any{"character_id": id, "file_path": asset.FilePath, "source": origin})
}

// speakVoiceSample has the character's saved voice say a line. It is one small paid
// speech call, made only when the owner asks for it.
func (s *Server) speakVoiceSample(ctx context.Context, project string, character *models.Character) ([]byte, error) {
	doc, err := s.database.GetVoiceDocument(project, "profile", character.ID)
	var profile VoiceProfile
	if err != nil || json.Unmarshal(doc.Data, &profile) != nil || !profile.Approved || !(voiceChoice{Provider: profile.Provider, VoiceID: profile.VoiceID}).usable() {
		return nil, fmt.Errorf("save a voice for %s first, or upload a recording", character.Name)
	}
	model, err := s.resolveSpeechModel(ctx, profile.Provider, profile.Model)
	if err != nil {
		return nil, err
	}
	key, endpoint, _, err := s.routes().Credentials(profile.Provider)
	if err != nil {
		return nil, fmt.Errorf("connect %s in Settings first: %w", profile.Provider, err)
	}
	text := strings.TrimSpace(character.SampleDialogue)
	if len(strings.Fields(text)) < 8 {
		text = strings.TrimSpace(text + " Hello, my name is " + character.Name + ". This is how I sound when I speak.")
	}
	return voiceoverSpeech(ctx, profile.Provider, key, endpoint, model, profile.VoiceID, text)
}

func (s *Server) handleDeleteVoiceSample(w http.ResponseWriter, r *http.Request) {
	project, ok := s.voiceProject(w, r)
	if !ok {
		return
	}
	asset, err := s.database.GetAsset(voiceSampleID(chi.URLParam(r, "character")))
	if err != nil || asset.Type != voiceSampleType || asset.ProjectID == nil || *asset.ProjectID != project {
		w.WriteHeader(204)
		return
	}
	if err = s.database.DeleteAsset(asset.ID); err != nil {
		http.Error(w, "failed to remove the voice sample", 500)
		return
	}
	if asset.CharacterID != nil {
		s.forgetDecision(project, "voice_sample:"+*asset.CharacterID)
	}
	if full, e := s.storagePath(asset.FilePath); e == nil {
		os.Remove(full)
	}
	w.WriteHeader(204)
}
