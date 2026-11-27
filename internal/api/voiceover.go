package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
	"videoflow-go/internal/seedance"

	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

// Voice-over from the script: each line spoken with its character's approved saved
// voice (or a narrator voice chosen in the editor). Speech is cached by voice+text, so
// pressing the button again never pays twice for a line that has not changed.
//
// A clip made with its own sound already has voices. The editor chooses:
//
//	keep     the video's own voices stay; only clips without sound (and typed lines)
//	         get voice lines. The default when the speech check found the clip's
//	         speech right.
//	replace  the clip's own sound is MUTED (never just lowered: two voices at once is
//	         the mistake) and each line is spoken when the clip's character actually
//	         says it, from what was heard in the clip. A voice line longer than its
//	         slot (until the next line, or the clip's end) is reported; with speed_up
//	         it is made up to 1.15× faster with ffmpeg atempo rather than overlapping.
//
// Lines from clips without sound, and lines that were not heard, keep the caption
// times (estimated per shot).

type voiceChoice struct {
	Provider string `json:"provider"`
	VoiceID  string `json:"voice_id"`
	Model    string `json:"model,omitempty"`
}

func (v voiceChoice) usable() bool {
	return speechProvider(v.Provider) && providers.SafeSpeechID(v.VoiceID)
}

type voiceoverItem struct {
	AssetID  string  `json:"asset_id"`
	Start    float64 `json:"start"`
	Duration float64 `json:"duration"`
	// OriginalDuration is the whole voice file; Duration may stop a hair short of it
	// (encoder padding) to fit its slot.
	OriginalDuration float64 `json:"original_duration"`
	Text             string  `json:"text"`
	Speaker          string  `json:"speaker"`
	Cached           bool    `json:"cached"`
	// Timing is "heard" when placed where the clip's character says the line.
	Timing string `json:"timing,omitempty"`
	// Slot is how long the line may last before the next line or its clip's end; Tempo
	// is the speed-up applied to fit it (1 = none).
	Slot  float64 `json:"slot,omitempty"`
	Tempo float64 `json:"tempo,omitempty"`
	// clipEnd is where the clip that says the line ends on the timeline (0: a typed line).
	clipEnd float64
}

// Voice-over modes.
const (
	voiceKeepNative    = "keep"
	voiceReplaceNative = "replace"
)

// voiceTempoLimit is the most a voice line is sped up to fit its slot.
const voiceTempoLimit = 1.15

// voiceGap is the pause kept between two voice lines.
const voiceGap = 0.15

// voiceoverSpeech is the one paid call; tests replace it.
var voiceoverSpeech = func(ctx context.Context, provider, key, endpoint, model, voice, text string) ([]byte, error) {
	if provider == "gemini" {
		wav, err := providers.GeminiSpeech(ctx, key, endpoint, model, voice, text)
		if err != nil {
			return nil, err
		}
		return wavToMP3(ctx, wav)
	}
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
		// Mode is keep or replace (the clips' own voices); empty is replace.
		Mode string `json:"mode"`
		// SpeedUp makes a line longer than its slot up to 1.15× faster.
		SpeedUp bool `json:"speed_up"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body) != nil {
		http.Error(w, "invalid voice-over request", 400)
		return
	}
	if body.Mode == "" {
		body.Mode = voiceReplaceNative
	}
	if body.Mode != voiceKeepNative && body.Mode != voiceReplaceNative {
		http.Error(w, "choose keep (the video's own voices) or replace", 400)
		return
	}
	cues := []clipCue{}
	skipped := []map[string]string{}
	native, keptNative := []string{}, 0
	for _, clip := range body.Clips {
		if !strings.HasPrefix(clip.AssetID, "output-") {
			continue // Images and uploaded media have no script.
		}
		c, err := s.clipCues(r.Context(), project, clip)
		if err != nil {
			skipped = append(skipped, map[string]string{"reason": err.Error()})
			continue
		}
		if len(c) > 0 && c[0].Native {
			native = append(native, clip.ID)
			if body.Mode == voiceKeepNative {
				keptNative += len(c) // the clip's own voices say these
				continue
			}
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
		cues = append(cues, clipCue{Cue: composition.Cue{ID: fmt.Sprintf("typed-%d", i), Start: line.Start, Text: text}, SpokenStart: -1, SpokenEnd: -1})
	}
	// A line is spoken when the clip's character says it, where that was heard.
	for i := range cues {
		if cues[i].SpokenStart >= 0 {
			cues[i].Cue.Start = cues[i].SpokenStart
		}
	}
	sort.SliceStable(cues, func(i, j int) bool { return cues[i].Cue.Start < cues[j].Cue.Start })
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
	for _, placed := range cues {
		cue := placed.Cue
		if cue.Scripted != "" {
			cue.Text = cue.Scripted // a voice says the script, whatever the clip said
		}
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
		item.clipEnd = placed.ClipEnd
		if placed.SpokenStart >= 0 {
			item.Timing = timingHeard
		}
		items = append(items, item)
	}
	tooLong, spedUp := []map[string]any{}, 0
	for _, i := range fitVoiceover(items) {
		need := neededTempo(items[i].Duration, items[i].Slot)
		if body.SpeedUp {
			faster, err := s.voiceoverFaster(r.Context(), project, items[i], min(need, voiceTempoLimit))
			if err != nil {
				writeJSON(w, 502, map[string]any{"error": err.Error(), "items": items, "skipped": skipped, "generated": generated, "reused": reused})
				return
			}
			items[i], spedUp = faster, spedUp+1
			if items[i].Duration <= items[i].Slot+0.001 {
				continue
			}
		}
		tooLong = append(tooLong, map[string]any{"text": items[i].Text, "speaker": items[i].Speaker, "start": items[i].Start, "duration": items[i].Duration, "slot": items[i].Slot, "tempo_needed": need, "fits_with_speed_up": need <= voiceTempoLimit})
	}
	shifted, overrun := spaceVoiceover(items, body.Clips)
	mute := []string{}
	if body.Mode == voiceReplaceNative {
		mute = native
	}
	writeJSON(w, 200, map[string]any{"items": items, "skipped": skipped, "generated": generated, "reused": reused, "shifted": shifted, "overrun": overrun,
		"mode": body.Mode, "native_clip_ids": native, "mute_clip_ids": mute, "kept_native": keptNative, "too_long": tooLong, "sped_up": spedUp, "speed_up_limit": voiceTempoLimit})
}

// handleVoiceoverStatus (POST /compositions/voiceover/status {clips}) says which clips
// of an edit speak with their own voices and what the speech check found, so the
// editor can offer "Keep the video's own voices" by default only when every one of
// them was heard saying its lines. Free: it reads stored checks, never listens.
func (s *Server) handleVoiceoverStatus(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	var body struct {
		Clips []composition.Clip `json:"clips"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body) != nil {
		http.Error(w, "invalid clips", 400)
		return
	}
	type clipSpeech struct {
		ClipID   string `json:"clip_id"`
		OutputID string `json:"output_id"`
		// Check is passed, flagged, kept or unchecked.
		Check   string `json:"check"`
		Finding string `json:"finding,omitempty"`
	}
	native := []clipSpeech{}
	verified, flagged := 0, 0
	for _, clip := range body.Clips {
		id, ok := strings.CutPrefix(clip.AssetID, "output-")
		if !ok {
			continue
		}
		out, err := s.database.GetRenderOutput(id)
		if err != nil {
			continue
		}
		job, err := s.database.GetRenderJob(out.RenderJobID)
		if err != nil || job.ProjectID == nil || *job.ProjectID != project || !nativeSound(job) {
			continue
		}
		c := clipSpeech{ClipID: clip.ID, OutputID: id, Check: "unchecked"}
		if record := storedAudioCheck(out.QAJSON); record != nil && !record.Result.Unavailable {
			switch {
			case record.AcceptedAt != nil:
				c.Check = "kept"
				verified++
			case record.passes():
				c.Check = "passed"
				verified++
			default:
				c.Check = "flagged"
				flagged++
				if len(record.Result.Issues) > 0 {
					c.Finding = record.Result.Issues[0].Detail
				}
			}
		}
		native = append(native, c)
	}
	recommended := voiceReplaceNative
	if len(native) > 0 && verified == len(native) {
		recommended = voiceKeepNative
	}
	writeJSON(w, 200, map[string]any{"native": native, "verified": verified, "flagged": flagged, "unchecked": len(native) - verified - flagged, "recommended": recommended})
}

// fitVoiceover works out each line's slot (until the next line starts, less a short
// pause, or its clip ends) and returns the lines longer than their slot. Items are
// sorted by start first.
func fitVoiceover(items []voiceoverItem) []int {
	sort.SliceStable(items, func(i, j int) bool { return items[i].Start < items[j].Start })
	long := []int{}
	for i := range items {
		end := math.Inf(1)
		if i+1 < len(items) {
			end = items[i+1].Start - voiceGap
		}
		if items[i].clipEnd > 0 {
			end = min(end, items[i].clipEnd)
		}
		if math.IsInf(end, 1) {
			continue
		}
		items[i].Slot = round3(max(0, end-items[i].Start))
		if items[i].Duration > items[i].Slot+0.001 {
			long = append(long, i)
		}
	}
	return long
}

// neededTempo is how much faster a line must be to fit its slot, rounded up to the
// thousandth so the faster line is never longer than the slot.
func neededTempo(duration, slot float64) float64 {
	if slot <= 0 {
		return math.Inf(1)
	}
	return math.Ceil(duration/slot*1000-1e-9) / 1000
}

// voiceoverFaster makes a voice line tempo× faster (pitch kept, ffmpeg atempo) and
// keeps it as its own voice file, so asking again reuses it.
func (s *Server) voiceoverFaster(ctx context.Context, project string, item voiceoverItem, tempo float64) (voiceoverItem, error) {
	if tempo <= 1 {
		return item, nil
	}
	id := "voiceover_" + compositionHash([]byte(fmt.Sprintf("%s\x00atempo=%.3f", item.AssetID, tempo)))[:20]
	faster := item
	faster.AssetID, faster.Tempo = id, tempo
	fit := func(seconds float64) voiceoverItem {
		// MP3 frames pad the end by a few milliseconds: stop at the slot rather than
		// run into the next line, never past the file.
		faster.OriginalDuration = round3(seconds)
		faster.Duration = faster.OriginalDuration
		if faster.Slot > 0 && faster.Duration > faster.Slot && faster.Duration-faster.Slot <= 0.05 {
			faster.Duration = faster.Slot
		}
		return faster
	}
	if a, err := s.database.GetAsset(id); err == nil && a.ProjectID != nil && *a.ProjectID == project {
		if full, err := s.storagePath(a.FilePath); err == nil {
			if d, err := voiceRequireMedia(ctx, full, "audio"); err == nil {
				faster.Cached = true
				return fit(d), nil
			}
		}
	}
	source, err := s.database.GetAsset(item.AssetID)
	if err != nil {
		return item, fmt.Errorf("the voice line to speed up is missing")
	}
	in, err := s.storagePath(source.FilePath)
	if err != nil {
		return item, err
	}
	dir := filepath.Join(s.cfg.StorageRoot, "assets")
	f, err := os.CreateTemp(dir, ".voiceover-*.mp3")
	if err != nil {
		return item, err
	}
	f.Close()
	defer os.Remove(f.Name())
	if err = atempoMP3(ctx, in, f.Name(), tempo); err != nil {
		return item, err
	}
	seconds, err := voiceRequireMedia(ctx, f.Name(), "audio")
	if err != nil {
		return item, fmt.Errorf("the faster voice line is unusable: %w", err)
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, id+".mp3")); err != nil {
		return item, err
	}
	var meta map[string]any
	_ = json.Unmarshal(source.MetadataJSON, &meta) // a source without notes still has its text below
	if meta == nil {
		meta = map[string]any{}
	}
	meta["voiceover"], meta["text"], meta["tempo"], meta["faster_than"], meta["seconds"] = true, item.Text, tempo, item.AssetID, round3(seconds)
	raw, _ := json.Marshal(meta)
	name := strings.TrimSuffix(source.Name, ".mp3") + fmt.Sprintf(" (%.2f×).mp3", tempo)
	a := &models.Asset{ID: id, ProjectID: &project, Type: "voiceover", Name: name, FilePath: "storage/assets/" + id + ".mp3", TagsJSON: []byte("[]"), MetadataJSON: raw}
	if _, err := s.database.GetAsset(id); err != nil {
		if err = s.database.CreateUploadedAsset(a); err != nil {
			return item, err
		}
	}
	return fit(seconds), nil
}

// atempoMP3 speeds speech up without changing its pitch.
func atempoMP3(ctx context.Context, in, out string, tempo float64) error {
	if !(tempo > 1 && tempo <= voiceTempoLimit) {
		return fmt.Errorf("a voice line is sped up at most %.2f×", voiceTempoLimit)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var problem bytes.Buffer
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-nostdin", "-protocol_whitelist", "file", "-i", in, "-vn", "-filter:a", fmt.Sprintf("atempo=%.3f", tempo), "-ac", "1", "-ar", "44100", "-b:a", "128k", "-f", "mp3", "-y", out)
	cmd.Stderr = &problem
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not speed up the voice line; install FFmpeg (%s)", strings.TrimSpace(problem.String()))
	}
	return nil
}

// spaceVoiceover moves lines later so one voice never talks over the previous line
// (a slow voice can outlast the caption slot), and reports how far the voice now
// runs past the end of the video clips.
func spaceVoiceover(items []voiceoverItem, clips []composition.Clip) (shifted int, overrun float64) {
	sort.SliceStable(items, func(i, j int) bool { return items[i].Start < items[j].Start })
	for i := 1; i < len(items); i++ {
		if end := items[i-1].Start + items[i-1].Duration + voiceGap; items[i].Start < end-0.0005 {
			items[i].Start = round3(end)
			shifted++
		}
	}
	videoEnd := 0.0
	for _, c := range clips {
		videoEnd = max(videoEnd, c.Start+c.Duration)
	}
	if n := len(items); n > 0 && videoEnd > 0 {
		overrun = max(0, round3(items[n-1].Start+items[n-1].Duration-videoEnd))
	}
	return shifted, overrun
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
				item.OriginalDuration = item.Duration
				return item, true, nil
			}
		}
	}
	key, endpoint, _, err := s.routes().Credentials(voice.Provider)
	if err != nil {
		return item, false, fmt.Errorf("connect %s in Settings first: %w", voice.Provider, err)
	}
	audio, err := voiceoverSpeech(ctx, voice.Provider, key, endpoint, model, voice.VoiceID, seedance.SpokenText(cue.Text))
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
	meta, _ := json.Marshal(map[string]any{"voiceover": true, "text": cue.Text, "character_id": cue.CharacterID, "provider": voice.Provider, "model": model, "voice_id": voice.VoiceID, "seconds": round3(duration)})
	a := &models.Asset{ID: id, ProjectID: &project, Type: "voiceover", Name: "Voice · " + speaker + ": " + label + ".mp3", FilePath: public, TagsJSON: []byte("[]"), MetadataJSON: meta}
	if _, err := s.database.GetAsset(id); err != nil {
		if err = s.database.CreateUploadedAsset(a); err != nil {
			return item, false, err
		}
	}
	item.Duration = round3(duration)
	item.OriginalDuration = item.Duration
	return item, false, nil
}

// wavToMP3 turns Google's WAV speech into the MP3 every saved voice line is kept as.
func wavToMP3(ctx context.Context, wav []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-protocol_whitelist", "pipe", "-f", "wav", "-i", "pipe:0", "-vn", "-ac", "1", "-ar", "44100", "-b:a", "128k", "-f", "mp3", "pipe:1")
	cmd.Stdin = bytes.NewReader(wav)
	var out, problem bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &problem
	if err := cmd.Run(); err != nil || out.Len() == 0 {
		return nil, fmt.Errorf("could not turn Google Gemini speech into MP3; install FFmpeg (%s)", strings.TrimSpace(problem.String()))
	}
	return out.Bytes(), nil
}
