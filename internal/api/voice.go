package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

// RegisterVoiceCaptionRoutes is called once during route setup. No action here
// starts an upstream request; only /run with an exact fingerprint does that.
func (s *Server) RegisterVoiceCaptionRoutes() {
	s.router.Get("/voice/state", s.handleVoiceState)
	s.router.Put("/voice/profiles/{character}", s.handleVoiceProfile)
	s.router.Put("/voice/scripts/{script}", s.handleSpokenScript)
	s.router.Post("/voice/tasks/prepare", s.handlePrepareVoice)
	s.router.Post("/voice/tasks/{task}/run", s.handleRunVoice)
	s.router.Post("/voice/tasks/{task}/refresh", s.handleRefreshVoice)
	s.router.Post("/voice/tasks/{task}/reconcile", s.handleReconcileVoice)
	s.router.Post("/voice/tasks/{task}/apply-captions", s.handleApplyScriptCaptions)
	s.router.Post("/voice/tasks/{task}/accept-voice", s.handleAcceptVoice)
	s.router.Get("/voice/outputs/{output}/captions/export", s.handleExportScriptCaptions)
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
func (s *Server) voiceAsset(project, id string) (*models.Asset, []byte, string, error) {
	a, err := s.database.GetAsset(id)
	if err != nil || a.ProjectID == nil || *a.ProjectID != project {
		return nil, nil, "", fmt.Errorf("reference asset does not belong to this project")
	}
	path, err := s.storagePath(a.FilePath)
	if err != nil {
		return nil, nil, "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (32<<20)+1))
	if err != nil || len(data) > 32<<20 {
		return nil, nil, "", fmt.Errorf("reference must be at most 32 MB")
	}
	return a, data, voiceHash(data), nil
}
func voiceDataURI(data []byte, kind string) string {
	return "data:" + kind + ";base64," + base64.StdEncoding.EncodeToString(data)
}
func voiceAudioKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".wav":
		return "audio/wav"
	case ".mp3":
		return "audio/mpeg"
	case ".flac":
		return "audio/flac"
	case ".m4a":
		return "audio/mp4"
	}
	return ""
}
func voiceInspect(ctx context.Context, path string) (float64, bool, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file,pipe", "-format_whitelist", "wav,mp3,flac,mov,matroska,webm", "-show_entries", "format=duration:stream=codec_type", "-of", "json", path).Output()
	if err != nil || len(raw) > 1<<20 {
		return 0, false, false, fmt.Errorf("FFprobe could not inspect media; install FFmpeg and provide valid audio/video")
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
		return 0, false, false, fmt.Errorf("invalid media metadata")
	}
	seconds, err := strconv.ParseFloat(info.Format.Duration, 64)
	if err != nil || seconds <= 0 || math.IsInf(seconds, 0) || math.IsNaN(seconds) {
		return 0, false, false, fmt.Errorf("invalid media duration")
	}
	audio, video := false, false
	for _, stream := range info.Streams {
		audio = audio || stream.Kind == "audio"
		video = video || stream.Kind == "video"
	}
	return seconds, audio, video, nil
}
func voiceProbe(ctx context.Context, path string) (float64, error) {
	duration, _, _, err := voiceInspect(ctx, path)
	return duration, err
}
func voiceRequireMedia(ctx context.Context, path, kind string) (float64, error) {
	duration, audio, video, err := voiceInspect(ctx, path)
	if err != nil {
		return 0, err
	}
	if (kind == "video" && !video) || (kind == "audio" && (!audio || video)) {
		return 0, fmt.Errorf("expected %s media stream", kind)
	}
	return duration, nil
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
	scripts, err := s.database.ListVoiceDocuments(project, "script")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tasks, err := s.database.ListVoiceTasks(project)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]any{"project_id": project, "profiles": profiles, "scripts": scripts, "tasks": tasks, "default_mode": "saved", "clone_sample_min_seconds": 10, "native_sample_max_seconds": 15, "clone_retention": "Use a cloned voice with TTS within 7 days to retain it.", "models": map[string]string{"clone": providers.FalVoiceCloneModel, "speech": providers.FalSpeechModel, "alignment": providers.FalAlignmentModel, "native_reference": providers.FalNativeVoiceModel, "lipsync": providers.FalLipSyncModel}})
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
	p.SampleHash = ""
	if p.Mode == "" {
		p.Mode = "saved"
	}
	if p.Mode != "saved" && p.Mode != "native_reference" {
		http.Error(w, "voice mode must be saved or native_reference", 400)
		return
	}
	if p.SampleAssetID != "" {
		a, _, hash, err := s.voiceAsset(project, p.SampleAssetID)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if voiceAudioKind(a.FilePath) == "" {
			http.Error(w, "sample must be an uploaded WAV, MP3, FLAC or M4A audio asset", 400)
			return
		}
		p.SampleHash = hash
	}
	if p.Mode == "native_reference" && p.SampleAssetID == "" {
		http.Error(w, "native reference mode requires an actual audio sample", 400)
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
func (s *Server) handleSpokenScript(w http.ResponseWriter, r *http.Request) {
	project, ok := s.voiceProject(w, r)
	if !ok {
		return
	}
	var body struct {
		Revision int          `json:"revision"`
		Data     SpokenScript `json:"data"`
	}
	if !voiceDecode(w, r, &body) {
		return
	}
	if len(body.Data.Lines) == 0 || len(body.Data.Lines) > 1000 {
		http.Error(w, "supply 1–1000 spoken lines; scene descriptions are not dialogue", 400)
		return
	}
	ids := map[string]bool{}
	for _, line := range body.Data.Lines {
		if line.ID == "" || ids[line.ID] || strings.TrimSpace(line.Text) == "" || line.Language == "" || alignmentLetters(line.Text) == "" {
			http.Error(w, "each spoken line needs a unique stable ID, speaker, language and exact text", 400)
			return
		}
		ids[line.ID] = true
		if err := s.voiceCharacter(project, line.CharacterID); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
	}
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	saved, err := s.database.SaveVoiceDocument(project, "script", chi.URLParam(r, "script"), body.Revision, voiceJSON(body.Data))
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	writeJSON(w, 200, saved)
}

type voicePreparation struct {
	Kind          string   `json:"kind"`
	ScriptID      string   `json:"script_id"`
	LineID        string   `json:"line_id"`
	CharacterID   string   `json:"character_id"`
	OutputID      string   `json:"output_id"`
	AudioAssetID  string   `json:"audio_asset_id"`
	ImageAssetIDs []string `json:"image_asset_ids"`
	Prompt        string   `json:"prompt"`
	Duration      int      `json:"duration"`
	AspectRatio   string   `json:"aspect_ratio"`
}

func (s *Server) voiceOwnedOutput(project, id string) (*models.RenderOutput, error) {
	out, err := s.database.GetRenderOutput(id)
	if err != nil {
		return nil, fmt.Errorf("output not found")
	}
	job, err := s.database.GetRenderJob(out.RenderJobID)
	if err != nil || job.ProjectID == nil || *job.ProjectID != project {
		return nil, fmt.Errorf("output does not belong to this project")
	}
	return out, nil
}
func (s *Server) voiceOutputBinding(project, id string) (db.VoiceBinding, string, error) {
	out, err := s.voiceOwnedOutput(project, id)
	if err != nil {
		return db.VoiceBinding{}, "", err
	}
	path, err := s.storagePath(out.VideoPath)
	if err != nil {
		return db.VoiceBinding{}, "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return db.VoiceBinding{}, "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return db.VoiceBinding{}, "", err
	}
	return db.VoiceBinding{Kind: "output", ID: id, Hash: hex.EncodeToString(hash.Sum(nil))}, path, nil
}
func (s *Server) voiceScript(project, id string) (*db.VoiceDocument, SpokenScript, error) {
	doc, err := s.database.GetVoiceDocument(project, "script", id)
	var script SpokenScript
	if err == nil {
		err = json.Unmarshal(doc.Data, &script)
	}
	if err == nil && !script.Approved {
		err = fmt.Errorf("approve the exact spoken script before preparing paid work")
	}
	return doc, script, err
}
func (s *Server) prepareVoice(ctx context.Context, project string, p voicePreparation) (*db.VoiceTask, error) {
	_, endpoint, _, err := s.routes().Credentials("fal")
	if err != nil {
		return nil, err
	}
	task := db.VoiceTask{ID: uuid.NewString(), ProjectID: project, Kind: p.Kind, Endpoint: endpoint, Status: "draft", Bindings: []db.VoiceBinding{}}
	input := map[string]any{}
	var script SpokenScript
	if p.Kind != "clone" {
		doc, spoken, err := s.voiceScript(project, p.ScriptID)
		if err != nil {
			return nil, err
		}
		script = spoken
		task.Bindings = append(task.Bindings, db.VoiceBinding{Kind: "script", ID: doc.ID, Revision: doc.Revision})
	}
	var line SpokenLine
	if p.Kind == "speech" || p.Kind == "native_reference" || p.Kind == "lipsync" {
		for _, l := range script.Lines {
			if l.ID == p.LineID {
				line = l
				break
			}
		}
		if line.ID == "" {
			return nil, fmt.Errorf("select a stable spoken line from the approved script")
		}
		p.CharacterID = line.CharacterID
		task.Bindings = append(task.Bindings, db.VoiceBinding{Kind: "utterance", ID: line.ID, Hash: voiceHash([]byte(line.Text))})
	}
	bindAsset := func(id string) (*models.Asset, []byte, error) {
		a, b, hash, err := s.voiceAsset(project, id)
		if err == nil {
			identity, identityErr := voiceAssetIdentity(a)
			if identityErr != nil {
				return nil, nil, identityErr
			}
			task.Bindings = append(task.Bindings, db.VoiceBinding{Kind: "asset", ID: id, Hash: hash}, db.VoiceBinding{Kind: "asset_identity", ID: id, Hash: voiceHash(identity)})
		}
		return a, b, err
	}
	var profile VoiceProfile
	if p.Kind == "clone" || p.Kind == "speech" || p.Kind == "native_reference" {
		if err := s.voiceCharacter(project, p.CharacterID); err != nil {
			return nil, err
		}
		character, _ := s.database.GetCharacter(p.CharacterID)
		task.Bindings = append(task.Bindings, db.VoiceBinding{Kind: "character", ID: p.CharacterID, Hash: voiceHash(voiceJSON(character))})
		doc, err := s.database.GetVoiceDocument(project, "profile", p.CharacterID)
		if err != nil {
			return nil, fmt.Errorf("set up an approved character voice first")
		}
		if err = json.Unmarshal(doc.Data, &profile); err != nil || !profile.Approved {
			return nil, fmt.Errorf("approve the character voice profile first")
		}
		task.Bindings = append(task.Bindings, db.VoiceBinding{Kind: "profile", ID: p.CharacterID, Revision: doc.Revision})
	}
	bindSample := func() (*models.Asset, []byte, error) {
		a, data, err := bindAsset(profile.SampleAssetID)
		if err != nil {
			return nil, nil, err
		}
		if voiceHash(data) != profile.SampleHash {
			return nil, nil, fmt.Errorf("voice sample changed since approval; review and save the character profile again")
		}
		return a, data, nil
	}
	switch p.Kind {
	case "clone":
		if profile.Mode != "saved" {
			return nil, fmt.Errorf("voice creation requires saved-voice mode")
		}
		a, data, err := bindSample()
		if err != nil {
			return nil, err
		}
		path, _ := s.storagePath(a.FilePath)
		duration, err := voiceRequireMedia(ctx, path, "audio")
		if err != nil {
			return nil, err
		}
		if duration < 10 || duration > 300 {
			return nil, fmt.Errorf("voice clone sample must be 10–300 seconds")
		}
		task.Model = providers.FalVoiceCloneModel
		input = map[string]any{"audio_url": voiceDataURI(data, voiceAudioKind(a.FilePath)), "model": "speech-02-hd"}
	case "speech":
		if profile.Mode != "saved" || profile.VoiceID == "" {
			return nil, fmt.Errorf("saved voice is missing; create/bind a voice or explicitly select native reference mode")
		}
		if profile.SampleAssetID != "" {
			if _, _, err := bindSample(); err != nil {
				return nil, err
			}
		}
		task.Model = providers.FalSpeechModel
		input = map[string]any{"text": line.Text, "voice_setting": map[string]any{"voice_id": profile.VoiceID}, "output_format": "url"}
	case "native_reference":
		if profile.Mode != "native_reference" {
			return nil, fmt.Errorf("explicitly select native reference mode on this character")
		}
		a, data, err := bindSample()
		if err != nil {
			return nil, err
		}
		path, _ := s.storagePath(a.FilePath)
		duration, err := voiceRequireMedia(ctx, path, "audio")
		if err != nil {
			return nil, err
		}
		kind := voiceAudioKind(a.FilePath)
		if duration > 15 || len(data) > 15<<20 || (kind != "audio/wav" && kind != "audio/mpeg") {
			return nil, fmt.Errorf("native reference audio must be WAV/MP3, at most 15 seconds and 15 MB")
		}
		images := []string{}
		imageIdentities := []map[string]any{}
		if len(p.ImageAssetIDs) < 1 || len(p.ImageAssetIDs) > 9 {
			return nil, fmt.Errorf("native reference mode needs 1–9 approved reference images")
		}
		for _, id := range p.ImageAssetIDs {
			a, b, err := bindAsset(id)
			if err != nil {
				return nil, err
			}
			if !models.ReferenceApproved(a.MetadataJSON) {
				return nil, fmt.Errorf("native reference image %s needs explicit consistency approval before reuse; review it in Assets", a.ID)
			}
			kind := http.DetectContentType(b)
			if !strings.HasPrefix(kind, "image/") || len(b) > 30<<20 {
				return nil, fmt.Errorf("native image reference must be an image under 30 MB")
			}
			identity, err := voiceAssetIdentity(a)
			if err != nil {
				return nil, err
			}
			imageIdentities = append(imageIdentities, map[string]any{"reference": fmt.Sprintf("@Image%d", len(images)+1), "identity": identity})
			images = append(images, voiceDataURI(b, kind))
		}
		if p.Duration < 4 || p.Duration > 15 {
			return nil, fmt.Errorf("native reference video duration must be 4–15 seconds")
		}
		if p.AspectRatio == "" {
			p.AspectRatio = "16:9"
		}
		if !strings.Contains("|16:9|9:16|1:1|4:3|3:4|21:9|", "|"+p.AspectRatio+"|") {
			return nil, fmt.Errorf("unsupported aspect ratio")
		}
		if strings.TrimSpace(p.Prompt) == "" {
			return nil, fmt.Errorf("provide a reviewed video prompt")
		}
		task.Model = providers.FalNativeVoiceModel
		input = map[string]any{"prompt": p.Prompt + "\nAuthoritative approved visual identities: preserve every locked trait, object kind, color, description and reference binding exactly; these constraints outrank scene suggestions: " + string(voiceJSON(imageIdentities)) + "\nUse @Audio1 as the speaker voice reference. Exact approved spoken line: " + line.Text, "audio_urls": []string{voiceDataURI(data, kind)}, "image_urls": images, "duration": strconv.Itoa(p.Duration), "aspect_ratio": p.AspectRatio, "resolution": "720p", "generate_audio": true}
	case "alignment", "lipsync":
		binding, path, err := s.voiceOutputBinding(project, p.OutputID)
		if err != nil {
			return nil, err
		}
		task.Bindings = append(task.Bindings, binding)
		if p.Kind == "alignment" {
			duration, err := voiceProbe(ctx, path)
			if err != nil {
				return nil, err
			}
			if duration > 600 {
				return nil, fmt.Errorf("local alignment supports final outputs up to 10 minutes")
			}
			temp, err := os.CreateTemp("", "videoflow-align-*.wav")
			if err != nil {
				return nil, err
			}
			name := temp.Name()
			temp.Close()
			defer os.Remove(name)
			bounded, cancel := context.WithTimeout(ctx, time.Minute)
			defer cancel()
			if raw, err := exec.CommandContext(bounded, "ffmpeg", "-v", "error", "-nostdin", "-y", "-protocol_whitelist", "file,pipe", "-format_whitelist", "wav,mp3,flac,mov,matroska,webm", "-i", path, "-vn", "-ac", "1", "-ar", "24000", "-c:a", "pcm_s16le", name).CombinedOutput(); err != nil {
				return nil, fmt.Errorf("extract final spoken audio: %v %s", err, raw)
			}
			audio, err := os.ReadFile(name)
			if err != nil {
				return nil, err
			}
			texts := []string{}
			for _, l := range script.Lines {
				texts = append(texts, l.Text)
			}
			task.Model = providers.FalAlignmentModel
			input = map[string]any{"audio_url": voiceDataURI(audio, "audio/wav"), "text": strings.Join(texts, "\n")}
		} else {
			a, audio, err := bindAsset(p.AudioAssetID)
			if err != nil {
				return nil, err
			}
			if voiceAudioKind(a.FilePath) == "" {
				return nil, fmt.Errorf("lip sync requires a speech audio asset")
			}
			var meta struct {
				ScriptID       string            `json:"script_id"`
				ScriptRevision int               `json:"script_revision"`
				LineID         string            `json:"line_id"`
				SourceBindings []db.VoiceBinding `json:"source_bindings"`
			}
			json.Unmarshal(a.MetadataJSON, &meta)
			if meta.ScriptID != p.ScriptID || meta.LineID != p.LineID || meta.ScriptRevision != voiceBinding(&task, "script").Revision {
				return nil, fmt.Errorf("speech audio is stale or belongs to another spoken line; generate it from this approved script revision")
			}
			if len(meta.SourceBindings) == 0 {
				return nil, fmt.Errorf("speech audio has no approved voice provenance")
			}
			if err := s.checkVoiceBindings(&db.VoiceTask{ProjectID: project, Bindings: meta.SourceBindings}); err != nil {
				return nil, fmt.Errorf("speech voice source is stale: %w", err)
			}
			task.Bindings = append(task.Bindings, meta.SourceBindings...)
			videoDuration, err := voiceProbe(ctx, path)
			if err != nil {
				return nil, err
			}
			audioPath, _ := s.storagePath(a.FilePath)
			audioDuration, err := voiceRequireMedia(ctx, audioPath, "audio")
			if err != nil {
				return nil, err
			}
			if math.Abs(videoDuration-audioDuration) > 0.25 {
				return nil, fmt.Errorf("audio/video duration differs by more than 250ms; edit timing before lip sync")
			}
			video, err := os.ReadFile(path)
			if err != nil || len(video) > 50<<20 {
				return nil, fmt.Errorf("lip sync video must be at most 50 MB")
			}
			task.Model = providers.FalLipSyncModel
			input = map[string]any{"video_url": voiceDataURI(video, "video/mp4"), "audio_url": voiceDataURI(audio, voiceAudioKind(a.FilePath)), "sync_mode": "cut_off"}
		}
	default:
		return nil, fmt.Errorf("kind must be clone, speech, native_reference, alignment or lipsync")
	}
	if err = providers.ValidateVoiceInput(task.Model, input); err != nil {
		return nil, err
	}
	task.Input = voiceJSON(input)
	// Bind semantic parameters as well as source revisions. Repeated prepare calls
	// recover the same task, including after a lost HTTP response or process restart.
	task.Fingerprint = voiceHash(voiceJSON(map[string]any{"kind": task.Kind, "model": task.Model, "endpoint": task.Endpoint, "input": input, "bindings": task.Bindings}))
	if err = s.checkVoiceBindings(&task); err != nil {
		return nil, err
	}
	return s.database.CreateVoiceTask(task)
}
func (s *Server) handlePrepareVoice(w http.ResponseWriter, r *http.Request) {
	project, ok := s.voiceProject(w, r)
	if !ok {
		return
	}
	var p voicePreparation
	if !voiceDecode(w, r, &p) {
		return
	}
	task, err := s.prepareVoice(r.Context(), project, p)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, 200, task)
}
func (s *Server) checkVoiceBindings(task *db.VoiceTask) error {
	for _, b := range task.Bindings {
		switch b.Kind {
		case "profile", "script":
			doc, err := s.database.GetVoiceDocument(task.ProjectID, b.Kind, b.ID)
			if err != nil || doc.Revision != b.Revision {
				return fmt.Errorf("%s revision changed; prepare and review a new action", b.Kind)
			}
		case "character":
			character, err := s.database.GetCharacter(b.ID)
			if err != nil || character.ProjectID == nil || *character.ProjectID != task.ProjectID || voiceHash(voiceJSON(character)) != b.Hash {
				return fmt.Errorf("character identity or voice rules changed; prepare and review a new action")
			}
		case "asset_identity":
			asset, err := s.database.GetAsset(b.ID)
			if err != nil || asset.ProjectID == nil || *asset.ProjectID != task.ProjectID {
				return fmt.Errorf("reference identity is unavailable")
			}
			identity, err := voiceAssetIdentity(asset)
			if err != nil || voiceHash(identity) != b.Hash {
				return fmt.Errorf("reference identity or locked traits changed; prepare and review a new action")
			}
		case "asset":
			_, _, hash, err := s.voiceAsset(task.ProjectID, b.ID)
			if err != nil || hash != b.Hash {
				return fmt.Errorf("reference asset changed; prepare and review a new action")
			}
		case "output":
			current, _, err := s.voiceOutputBinding(task.ProjectID, b.ID)
			if err != nil || current.Hash != b.Hash {
				return fmt.Errorf("final audio/video changed; alignment or lip sync is stale")
			}
		}
	}
	return nil
}
func (s *Server) voiceTaskFromRequest(w http.ResponseWriter, r *http.Request) (*db.VoiceTask, bool) {
	project, ok := s.voiceProject(w, r)
	if !ok {
		return nil, false
	}
	task, err := s.database.GetVoiceTask(project, chi.URLParam(r, "task"))
	if err != nil {
		http.Error(w, "task not found in active project", 404)
		return nil, false
	}
	return task, true
}
func (s *Server) voiceClient(task *db.VoiceTask) (*providers.FalClient, error) {
	key, endpoint, _, err := s.routes().Credentials("fal")
	if err != nil {
		return nil, err
	}
	if endpoint != task.Endpoint {
		return nil, fmt.Errorf("fal endpoint changed; restore the captured connection before submitting/reconciling")
	}
	return providers.NewFalClientAt(key, endpoint, ""), nil
}
func (s *Server) handleRunVoice(w http.ResponseWriter, r *http.Request) {
	task, ok := s.voiceTaskFromRequest(w, r)
	if !ok {
		return
	}
	var body struct {
		Confirmed   bool   `json:"confirmed"`
		Fingerprint string `json:"fingerprint"`
	}
	if !voiceDecode(w, r, &body) {
		return
	}
	if !body.Confirmed || body.Fingerprint != task.Fingerprint {
		http.Error(w, "Run must confirm the exact prepared action fingerprint", 409)
		return
	}
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	if err := s.checkVoiceBindings(task); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	client, err := s.voiceClient(task)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err = s.database.ClaimVoiceTask(task.ProjectID, task.ID); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	var input map[string]any
	json.Unmarshal(task.Input, &input)
	id, err := client.SubmitVoice(r.Context(), task.Model, input)
	if err != nil {
		task.Status = "uncertain"
		task.Error = "Submission outcome uncertain. Review fal activity and reconcile an upstream request ID; this task will never automatically resubmit. " + err.Error()
	} else {
		task.UpstreamID = id
		task.Status = "queued"
		task.Error = ""
	}
	if err = s.database.UpdateVoiceTask(task); err != nil {
		http.Error(w, "could not save upstream receipt; submission is fenced, reconcile provider activity", 503)
		return
	}
	writeJSON(w, 200, task)
}
func (s *Server) handleReconcileVoice(w http.ResponseWriter, r *http.Request) {
	task, ok := s.voiceTaskFromRequest(w, r)
	if !ok {
		return
	}
	var body struct {
		UpstreamID string `json:"upstream_id"`
		Reviewed   bool   `json:"reviewed"`
	}
	if !voiceDecode(w, r, &body) {
		return
	}
	if !body.Reviewed || body.UpstreamID == "" || strings.ContainsAny(body.UpstreamID, "/\\?#") || len(body.UpstreamID) > 200 {
		http.Error(w, "review fal activity and provide its exact request ID", 400)
		return
	}
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	task, err := s.database.GetVoiceTask(task.ProjectID, task.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if (task.Status != "submitting" && task.Status != "uncertain") || task.UpstreamID != "" {
		http.Error(w, "only uncertain submissions without a saved receipt can be reconciled", 409)
		return
	}
	// Reconciliation deliberately only attaches a receipt; refresh will retrieve it.
	task.UpstreamID = body.UpstreamID
	task.Status = "queued"
	task.Error = "Manually reconciled provider receipt; review the returned artifact before reuse."
	if err = s.database.UpdateVoiceTask(task); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, task)
}
