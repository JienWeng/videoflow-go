package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"videoflow-go/internal/agents"
	"videoflow-go/internal/composition"
	"videoflow-go/internal/costs"
	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
)

// Sound modes. In "together" (the default) the video model makes each clip's picture and
// voices at once. In "voice_first" each clip's speech is made first, line by line with the
// characters' saved voices (the same voices and cache as the voice-over), joined into one
// dialogue track the owner can listen to and change; the clip is then made by a video
// model that takes voice recordings (Seedance through ArgoLink), lip-synced to that exact
// track, and the edit plays the track as the clip's sound. Google's models take no voice.
const (
	soundTogether   = "together"
	soundVoiceFirst = "voice_first"
	soundModeKey    = "sound_mode"
	// voiceTrackNarratorKey keeps the narrator voice of voice-first tracks.
	voiceTrackNarratorKey = "voice_track_narrator"
)

// Chat actions of voice first.
const (
	setSoundModeAction    = "set_sound_mode"
	makeVoiceTracksAction = "make_voice_tracks"
)

// voiceTrackLead is the silence before the first line; voiceTrackGap between two lines.
const (
	voiceTrackLead = 0.25
	voiceTrackGap  = 0.3
)

func voiceTrackVoiceKey(characterID string) string { return "voice_track_voice:" + characterID }

// soundMode is the project's sound mode.
func (s *Server) soundMode(project string) string {
	if d, err := s.database.GetDecision(project, soundModeKey); err == nil && d.Value == soundVoiceFirst {
		return soundVoiceFirst
	}
	return soundTogether
}

func validSoundMode(mode string) bool { return mode == soundTogether || mode == soundVoiceFirst }

func (s *Server) setSoundMode(project, mode, source string) error {
	if !validSoundMode(mode) {
		return fmt.Errorf("choose together or voice_first")
	}
	_, err := s.database.SetDecision(project, soundModeKey, mode, source, false)
	return err
}

// voiceVideoServices are the video services that take voice recordings with a clip.
func voiceVideoServices() []map[string]any {
	out := []map[string]any{}
	for _, h := range costs.Hosts {
		if h.Voices > 0 {
			out = append(out, map[string]any{"id": h.Name, "service": h.Label, "voice_recordings_per_clip": h.Voices})
		}
	}
	return out
}

func hostTakesVoice(provider string) bool {
	for _, h := range costs.Hosts {
		if h.Name == provider {
			return h.Voices > 0
		}
	}
	return false
}

// soundModeView is what the owner and the assistant are told about the sound mode.
func (s *Server) soundModeView(project string) map[string]any {
	provider, _ := s.routes().MediaSelection("video")
	return map[string]any{"sound_mode": s.soundMode(project), "video_service": videoServiceLabel(provider), "video_service_takes_voice": hostTakesVoice(provider), "voice_video_services": voiceVideoServices()}
}

// handleSoundMode (GET/PUT /project/sound-mode {sound_mode}) reads or sets the mode.
func (s *Server) handleSoundMode(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	if r.Method == http.MethodPut {
		var body struct {
			SoundMode string `json:"sound_mode"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || !validSoundMode(body.SoundMode) {
			http.Error(w, "choose together or voice_first", 400)
			return
		}
		if err = s.setSoundMode(project, body.SoundMode, db.DecisionByUser); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	writeJSON(w, 200, s.soundModeView(project))
}

// voiceTrackLine is one spoken line of a dialogue track and where it is in it.
type voiceTrackLine struct {
	LineID      string  `json:"line_id"`
	Text        string  `json:"text"`
	CharacterID string  `json:"character_id,omitempty"`
	Speaker     string  `json:"speaker"`
	AssetID     string  `json:"asset_id,omitempty"`
	Start       float64 `json:"start"`
	Duration    float64 `json:"duration"`
}

// voiceTrack is the dialogue track of one clip of a scene.
type voiceTrack struct {
	UnitIndex int      `json:"unit_index"`
	ShotIDs   []string `json:"shot_ids"`
	AssetID   string   `json:"asset_id,omitempty"`
	URL       string   `json:"url,omitempty"`
	Seconds   float64  `json:"seconds"`
	// ClipSeconds is how long the clip is planned to be in the edit.
	ClipSeconds int              `json:"clip_seconds"`
	Lines       []voiceTrackLine `json:"lines"`
	Ready       bool             `json:"ready"`
	Cached      bool             `json:"cached,omitempty"`
	// TooLong is set when the speech runs past the clip's planned length in the edit.
	TooLong bool `json:"too_long,omitempty"`
}

// unitSpokenLines are the scene's lines spoken in this clip: a line pinned to a shot
// (DialogueShots) is spoken in that shot's clip; a line pinned nowhere only when the
// scene is one clip.
func unitSpokenLines(scene *models.Scene, unit renderUnit, units int) []voiceTrackLine {
	settings := sceneProduction(scene)
	mine := map[string]bool{}
	for _, id := range unit.ShotIDs {
		mine[id] = true
	}
	out := []voiceTrackLine{}
	for i, line := range sceneSpokenLines(scene) {
		text := strings.TrimSpace(line.Text)
		if text == "" {
			continue
		}
		id := line.ID
		if id == "" {
			id = fmt.Sprintf("line-%d", i)
		}
		target := settings.DialogueShots[id]
		if (target == "" && units > 1) || (target != "" && !mine[target]) {
			continue
		}
		out = append(out, voiceTrackLine{LineID: id, Text: text, CharacterID: line.CharacterID})
	}
	return out
}

// lineVoice is the voice a voice-first line is spoken with: the one chosen for the track,
// else the character's approved saved voice, else the track's narrator voice.
func (s *Server) lineVoice(project, characterID string) (voiceChoice, string) {
	narrator := voiceChoice{}
	if d, err := s.database.GetDecision(project, voiceTrackNarratorKey); err == nil {
		_ = json.Unmarshal([]byte(d.Value), &narrator) // an unreadable choice is no narrator
	}
	if characterID == "" {
		return narrator, "Narrator"
	}
	name := "Narrator"
	if c, err := s.database.GetCharacter(characterID); err == nil && c.ProjectID != nil && *c.ProjectID == project {
		name = c.Name
	}
	if d, err := s.database.GetDecision(project, voiceTrackVoiceKey(characterID)); err == nil {
		var v voiceChoice
		if json.Unmarshal([]byte(d.Value), &v) == nil && v.usable() {
			return v, name
		}
	}
	if v := s.savedVoice(project, characterID); v.usable() {
		return v, name
	}
	return narrator, name
}

// savedVoice is the character's approved saved voice, or none.
func (s *Server) savedVoice(project, characterID string) voiceChoice {
	if doc, err := s.database.GetVoiceDocument(project, "profile", characterID); err == nil {
		var p VoiceProfile
		if json.Unmarshal(doc.Data, &p) == nil && p.Approved {
			return voiceChoice{Provider: p.Provider, VoiceID: p.VoiceID, Model: p.Model}
		}
	}
	return voiceChoice{}
}

// voiceTrack finds (or, with make, makes) the dialogue track of one clip. The track is
// named by what it says and with which voices, so a changed line or voice is a new track
// and an unchanged one is never paid for twice. Without make nothing is spent.
func (s *Server) voiceTrack(ctx context.Context, scene *models.Scene, unit renderUnit, units int, create bool) (voiceTrack, error) {
	project := *scene.ProjectID
	t := voiceTrack{UnitIndex: unit.Index, ShotIDs: unit.ShotIDs, ClipSeconds: unit.Length, Lines: unitSpokenLines(scene, unit, units)}
	if len(t.Lines) == 0 {
		t.Ready = true // nobody speaks in this clip
		return t, nil
	}
	language := sceneBrief(scene).Language
	sign := []string{project, scene.ID}
	voices := make([]voiceChoice, len(t.Lines))
	for i := range t.Lines {
		voices[i], t.Lines[i].Speaker = s.lineVoice(project, t.Lines[i].CharacterID)
		sign = append(sign, t.Lines[i].LineID, t.Lines[i].Text, t.Lines[i].CharacterID, voices[i].Provider, voices[i].Model, voices[i].VoiceID, language)
	}
	t.AssetID = "voicetrack_" + compositionHash([]byte(strings.Join(sign, "\x00")))[:20]
	if a, err := s.database.GetAsset(t.AssetID); err == nil && a.ProjectID != nil && *a.ProjectID == project {
		var meta struct {
			Seconds float64          `json:"seconds"`
			Lines   []voiceTrackLine `json:"lines"`
		}
		if json.Unmarshal(a.MetadataJSON, &meta) == nil && meta.Seconds > 0 {
			t.Seconds, t.Lines, t.Ready, t.Cached, t.URL = meta.Seconds, meta.Lines, true, true, "/"+a.FilePath
			t.TooLong = t.Seconds > float64(t.ClipSeconds)+0.05
			return t, nil
		}
	}
	if !create {
		t.AssetID = ""
		return t, nil
	}
	files := []string{}
	at := voiceTrackLead
	for i := range t.Lines {
		line := &t.Lines[i]
		if !voices[i].usable() {
			return t, fmt.Errorf("no voice for %s: set up %s's voice in Voices, or choose a narrator voice for the tracks", line.Speaker, line.Speaker)
		}
		item, _, err := s.voiceoverLine(ctx, project, voices[i], composition.Cue{ID: line.LineID, Text: line.Text, CharacterID: line.CharacterID, Language: language}, line.Speaker)
		if err != nil {
			return t, err
		}
		a, err := s.database.GetAsset(item.AssetID)
		if err != nil {
			return t, err
		}
		full, err := s.storagePath(a.FilePath)
		if err != nil {
			return t, err
		}
		files = append(files, full)
		line.AssetID, line.Start, line.Duration = item.AssetID, round3(at), item.Duration
		at += item.Duration + voiceTrackGap
	}
	dir := filepath.Join(s.cfg.StorageRoot, "assets")
	f, err := os.CreateTemp(dir, ".voicetrack-*.mp3")
	if err != nil {
		return t, err
	}
	f.Close()
	defer os.Remove(f.Name())
	if err = joinVoiceLines(ctx, files, f.Name()); err != nil {
		return t, err
	}
	seconds, err := voiceRequireMedia(ctx, f.Name(), "audio")
	if err != nil {
		return t, fmt.Errorf("the joined voice track is unusable: %w", err)
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, t.AssetID+".mp3")); err != nil {
		return t, err
	}
	t.Seconds, t.Ready = round3(seconds), true
	t.TooLong = t.Seconds > float64(t.ClipSeconds)+0.05
	meta, _ := json.Marshal(map[string]any{"voice_track": true, "scene_id": scene.ID, "shot_ids": unit.ShotIDs, "lines": t.Lines, "seconds": t.Seconds})
	public := "storage/assets/" + t.AssetID + ".mp3"
	t.URL = "/" + public
	asset := &models.Asset{ID: t.AssetID, ProjectID: &project, Type: "voice_track", Name: fmt.Sprintf("Voice track · %s · clip %d.mp3", scene.Title, unit.Index+1), FilePath: public, TagsJSON: []byte("[]"), MetadataJSON: meta}
	if _, err := s.database.GetAsset(t.AssetID); err != nil {
		if err = s.database.CreateUploadedAsset(asset); err != nil {
			return t, err
		}
	}
	return t, nil
}

// joinVoiceLines joins voice lines into one track: a short lead-in, then each line with a
// short pause after it.
func joinVoiceLines(ctx context.Context, files []string, out string) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	args := []string{"-v", "error", "-nostdin", "-protocol_whitelist", "file,lavfi", "-f", "lavfi", "-t", fmt.Sprintf("%.3f", voiceTrackLead), "-i", "anullsrc=r=44100:cl=mono"}
	filter := "[0:a]aformat=sample_rates=44100:channel_layouts=mono[a0];"
	labels := "[a0]"
	for i, file := range files {
		args = append(args, "-i", file)
		filter += fmt.Sprintf("[%d:a]aresample=44100,aformat=sample_rates=44100:channel_layouts=mono,apad=pad_dur=%.3f[a%d];", i+1, voiceTrackGap, i+1)
		labels += fmt.Sprintf("[a%d]", i+1)
	}
	filter += fmt.Sprintf("%sconcat=n=%d:v=0:a=1[out]", labels, len(files)+1)
	args = append(args, "-filter_complex", filter, "-map", "[out]", "-ac", "1", "-ar", "44100", "-b:a", "128k", "-f", "mp3", "-y", out)
	var problem bytes.Buffer
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stderr = &problem
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not join the voice lines; install FFmpeg (%s)", strings.TrimSpace(problem.String()))
	}
	return nil
}

// sceneVoiceTracks finds or makes the dialogue track of every clip of the scene's plan.
func (s *Server) sceneVoiceTracks(ctx context.Context, scene *models.Scene, create bool) ([]voiceTrack, error) {
	shots, err := s.database.ListShots(scene.ID)
	if err != nil {
		return nil, err
	}
	_, model, err := s.productionVideoModel(ctx)
	if err != nil {
		return nil, err
	}
	units, err := groupRenderUnits(shots, scene.Duration, sceneProduction(scene).Strategy, model.Durations)
	if err != nil {
		return nil, err
	}
	out := []voiceTrack{}
	for _, u := range units {
		t, err := s.voiceTrack(ctx, scene, u, len(units), create)
		if err != nil {
			return out, err
		}
		out = append(out, t)
	}
	return out, nil
}

// applyVoiceFirst makes a voice-first plan's clips cover their dialogue tracks and says
// what still stops them. It runs on the plan's units before the plan is named, so a
// changed track is a new plan of that scene only.
func (s *Server) applyVoiceFirst(ctx context.Context, scene *models.Scene, p *productionPlan, takesVoice int, durations []int) {
	if s.soundMode(*scene.ProjectID) != soundVoiceFirst || p.Settings.AudioMode != "native" || sceneBrief(scene).ConversationMode == "silent" {
		return
	}
	p.SoundMode = soundVoiceFirst
	missing := 0
	for i := range p.Units {
		u := &p.Units[i]
		t, _ := s.voiceTrack(ctx, scene, *u, len(p.Units), false) // never spends
		if !t.Ready {
			missing++
		}
		if t.Ready && t.Seconds > float64(u.Duration) {
			need := int(math.Ceil(t.Seconds))
			best := 0
			for _, d := range durations {
				if d >= need && (best == 0 || d < best) {
					best = d
				}
			}
			if best == 0 {
				p.voiceFirstBlock = fmt.Sprintf("The voice track of clip %d is %.1f seconds, longer than the longest clip the video model makes. Shorten its lines and make the voice tracks again.", i+1, t.Seconds)
			} else {
				u.Duration = best
			}
		}
		p.VoiceTracks = append(p.VoiceTracks, t)
	}
	switch {
	case takesVoice == 0:
		p.voiceFirstBlock = voiceFirstServiceBlock
	case missing > 0:
		p.voiceFirstBlock = "Voice first: make this scene's voice tracks first (make_voice_tracks), listen to them, then make the clips."
	}
}

const voiceFirstServiceBlock = "Voice first needs a video service that takes voice recordings (Seedance through ArgoLink); the one in use cannot. Switch video service, or set the sound mode back to together."

// handleSceneVoiceTracks (GET/POST /scenes/{id}/voice-tracks) reads a scene's dialogue
// tracks (free) or makes the missing ones in the background (speech credits; unchanged
// lines are reused). POST body: optional narrator voice and per-character voices,
// remembered for the project's tracks.
func (s *Server) handleSceneVoiceTracks(w http.ResponseWriter, r *http.Request) {
	scene, ok := s.productionScene(w, r)
	if !ok {
		return
	}
	project := *scene.ProjectID
	if r.Method == http.MethodGet {
		tracks, err := s.sceneVoiceTracks(r.Context(), scene, false)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		writeJSON(w, 200, map[string]any{"scene_id": scene.ID, "sound_mode": s.soundMode(project), "tracks": tracks})
		return
	}
	var body struct {
		Narrator *voiceChoice           `json:"narrator"`
		Voices   map[string]voiceChoice `json:"voices"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request body", 400)
		return
	}
	remember := func(key string, v voiceChoice) error {
		if !v.usable() {
			return fmt.Errorf("choose a saved voice (provider and voice id)")
		}
		raw, _ := json.Marshal(v)
		_, err := s.database.SetDecision(project, key, string(raw), db.DecisionByUser, false)
		return err
	}
	if body.Narrator != nil {
		if err := remember(voiceTrackNarratorKey, *body.Narrator); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
	}
	for id, v := range body.Voices {
		if c, err := s.database.GetCharacter(id); err != nil || c.ProjectID == nil || *c.ProjectID != project {
			http.Error(w, "character is not in this project", 404)
			return
		}
		if err := remember(voiceTrackVoiceKey(id), v); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
	}
	op := &models.Op{Kind: "voice_tracks", ProjectID: &project}
	s.operation(w, r, op, func(ctx context.Context) (any, error) {
		tracks, err := s.sceneVoiceTracks(ctx, scene, true)
		total := 0.0
		for _, t := range tracks {
			total += t.Seconds
		}
		return map[string]any{"scene_id": scene.ID, "tracks": tracks, "seconds": round3(total)}, err
	})
}

// voiceTrackOfJob is the dialogue track a voice-first clip was made to.
func (s *Server) voiceTrackOfJob(job models.RenderJob) (*models.Asset, float64) {
	var payload struct {
		VoiceTrack string `json:"voice_track"`
	}
	if json.Unmarshal(job.RequestJSON, &payload) != nil || payload.VoiceTrack == "" {
		return nil, 0
	}
	a, err := s.database.GetAsset(payload.VoiceTrack)
	if err != nil || a.ProjectID == nil || job.ProjectID == nil || *a.ProjectID != *job.ProjectID {
		return nil, 0
	}
	var meta struct {
		Seconds float64 `json:"seconds"`
	}
	_ = json.Unmarshal(a.MetadataJSON, &meta) // a track without notes plays to the clip's end
	return a, meta.Seconds
}

// attachVoiceTrack sends a voice-first clip its dialogue track as @Audio 1 instead of the
// speakers' voice samples. It reports whether the clip has one.
func (s *Server) attachVoiceTrack(payload map[string]any, project string) bool {
	id, _ := payload["voice_track"].(string)
	if id == "" {
		return false
	}
	a, err := s.database.GetAsset(id)
	if err != nil || a.ProjectID == nil || *a.ProjectID != project {
		delete(payload, "voice_track")
		return false
	}
	payload["audio_urls"] = []string{a.FilePath}
	return true
}

// voiceVideoSwitch is the routing request that moves videos to a usable service that
// takes voice recordings (want names one, or "" for the cheapest), keeping the video
// model in use when that service sells it, else Seedance 2.0. Nil when none can.
func (s *Server) voiceVideoSwitch(want string) map[string]any {
	settings, _ := s.database.AppSettingsSnapshot()
	current := s.chatVideoFamily(settings)
	want = strings.TrimSpace(want)
	if strings.EqualFold(want, "cheapest") {
		want = ""
	}
	for _, h := range costs.Hosts {
		if h.Voices == 0 || want != "" && !strings.EqualFold(want, h.Name) && !strings.EqualFold(want, h.Label) {
			continue
		}
		family := current
		if _, ok := h.Models[family]; !ok {
			family = "seedance-2.0"
		}
		for _, v := range s.videoOffers(family, "720p", 1, settings) {
			if v.Provider == h.Name && v.Usable && !v.InUse {
				return map[string]any{"mode": "manual", "model": family, "provider": v.Provider, "label": v.Label, "per_second": v.PerSecond, "takes_voice": true}
			}
		}
	}
	return nil
}

// chatSoundMode checks set_sound_mode. Voice first through a service that takes no
// voice carries the switch to one that does, on the same button; with none connected
// the reply says what to connect instead.
func (s *Server) chatSoundMode(project string, res *agents.ChatIntentResponse, intent map[string]any) string {
	mode := strings.TrimSpace(res.SoundMode)
	if !validSoundMode(mode) {
		intent["action"], intent["reply"] = "unknown", "Say which sound mode you want: together (the video model makes picture and voices at once) or voice first (the speech is made first and the clips lip-sync to it)."
		return "unknown"
	}
	intent["sound_mode"] = mode
	provider, _ := s.routes().MediaSelection("video")
	if mode != soundVoiceFirst || hostTakesVoice(provider) {
		return setSoundModeAction
	}
	if choice := s.voiceVideoSwitch(res.SwitchTo); choice != nil {
		intent["video_switch"] = choice
		return setSoundModeAction
	}
	intent["action"] = "unknown"
	intent["reply"] = videoServiceLabel(provider) + " can't take voice recordings, so voice first can't be used with it. Add an ArgoLink key in Settings (Seedance there takes the voice track), then ask me again."
	return "unknown"
}
