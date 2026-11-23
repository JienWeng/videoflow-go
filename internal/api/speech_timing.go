package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"videoflow-go/internal/costs"
	"videoflow-go/internal/db"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

// Captions timed by listening. A clip made with its own sound is spoken whenever the
// video model chose, not when the script planned, so the clip's audio is transcribed
// once with word times and each script line is placed where it is actually heard. The
// script still supplies the words; listening only supplies the times.

// listenToClip returns the words heard in a clip's sound and how many seconds were
// listened to. Tests replace it.
var listenToClip = func(s *Server, ctx context.Context, video, language, hint string) ([]providers.SpokenWord, float64, error) {
	listener := s.listener()
	key, endpoint, _, err := s.routes().Credentials(listener)
	if err != nil && strings.Contains(err.Error(), "not configured") {
		return nil, 0, errNoListener
	}
	if err != nil {
		return nil, 0, err
	}
	if strings.TrimSpace(key) == "" {
		return nil, 0, errNoListener
	}
	dir, err := os.MkdirTemp("", "videoflow-listen-*")
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(dir)
	wav := filepath.Join(dir, "clip.wav")
	if err = s.media.ExtractAudio(ctx, video, wav); err != nil {
		return nil, 0, err
	}
	audio, err := os.ReadFile(wav)
	if err != nil {
		return nil, 0, err
	}
	var words []providers.SpokenWord
	if listener == "gemini" {
		// Google's word timestamps cannot be combined with a vocabulary hint; the script
		// still supplies the words.
		words, err = providers.GeminiTranscribeWords(ctx, key, endpoint, audio, language)
	} else {
		words, err = providers.NewOpenAIImages(key, endpoint, "").TranscribeWords(ctx, audio, language, hint)
	}
	// 16 kHz mono 16-bit audio is 32,000 bytes a second.
	return words, float64(len(audio)) / 32000, err
}

type listenerError string

func (e listenerError) Error() string { return string(e) }

const errNoListener = listenerError("no OpenAI or Google Gemini key is saved, so captions keep their estimated times")

// listener is the service that listens to clips: the one chosen in AI setup, or with
// none chosen, OpenAI when its key is saved and otherwise Google Gemini when its key is.
func (s *Server) listener() string {
	routes := s.routes()
	if chosen := routes.Setting("listen_provider", ""); chosen == "openai" || chosen == "gemini" {
		return chosen
	}
	for _, name := range []string{"openai", "gemini"} {
		if key, _, _, err := routes.Credentials(name); err == nil && strings.TrimSpace(key) != "" {
			return name
		}
	}
	return "openai"
}

// speechCode is the language code the listener takes; empty lets it detect the language.
func speechCode(language string) string {
	l := strings.ToLower(strings.TrimSpace(language))
	for code, names := range map[string][]string{
		"en": {"en", "english"}, "zh": {"zh", "mandarin", "chinese", "cantonese", "中文", "普通话"}, "ms": {"ms", "malay", "bahasa"},
		"ja": {"ja", "japanese"}, "ko": {"ko", "korean"}, "es": {"es", "spanish"}, "fr": {"fr", "french"}, "de": {"de", "german"},
		"id": {"id", "indonesian"}, "hi": {"hi", "hindi"}, "ta": {"ta", "tamil"}, "th": {"th", "thai"}, "vi": {"vi", "vietnamese"},
		"pt": {"pt", "portuguese"}, "it": {"it", "italian"}, "ar": {"ar", "arabic"}, "ru": {"ru", "russian"},
	} {
		for _, name := range names {
			if l == name || strings.HasPrefix(l, name+" ") || strings.HasPrefix(l, name+"-") {
				return code
			}
		}
	}
	return ""
}

// clipHearing is what was heard in a clip, kept on it as qa_json.speech_timing so a
// clip is paid for once: the speech check and caption timing both read it.
type clipHearing struct {
	Video    string                 `json:"video"`
	Words    []providers.SpokenWord `json:"words"`
	Listener string                 `json:"listener,omitempty"`
	Model    string                 `json:"model,omitempty"`
	Seconds  float64                `json:"seconds,omitempty"`
	// Fresh is set when this call listened (and paid); it is not stored.
	Fresh bool `json:"-"`
}

// savedHearing is the clip's stored transcript when it is of the current video.
func savedHearing(notes json.RawMessage, video string) *clipHearing {
	var holder struct {
		Hearing *clipHearing `json:"speech_timing"`
	}
	if json.Unmarshal(notes, &holder) != nil || holder.Hearing == nil || holder.Hearing.Video != video || holder.Hearing.Words == nil {
		return nil
	}
	return holder.Hearing
}

// nativeSound says whether a clip was made with its own sound (voices included).
func nativeSound(job *models.RenderJob) bool {
	var request struct {
		Sound  bool   `json:"generate_audio"`
		Mode   string `json:"audio_mode"`
		Policy struct {
			Mode string `json:"mode"`
		} `json:"voice_policy"`
	}
	if job == nil || json.Unmarshal(job.RequestJSON, &request) != nil {
		return false
	}
	return request.Sound || request.Policy.Mode == "native" && request.Mode != "silent"
}

// clipTranscript returns the words spoken in a finished clip, listening to it the
// first time and remembering the answer on the clip. The language is left to the
// listener to detect, so a clip spoken in the wrong language is heard as it is; hint
// is the expected script, which keeps names and spellings (and the Chinese character
// set) the way they are written. errNoListener means no listening key is saved.
func (s *Server) clipTranscript(ctx context.Context, out *models.RenderOutput, job *models.RenderJob, hint string) (clipHearing, error) {
	if saved := savedHearing(out.QAJSON, out.VideoPath); saved != nil {
		return *saved, nil
	}
	unlock, err := s.lockSelfCheck(ctx, "listen:"+out.ID)
	if err != nil {
		return clipHearing{}, err
	}
	defer unlock()
	if fresh, e := s.database.GetRenderOutput(out.ID); e == nil { // listened meanwhile
		if saved := savedHearing(fresh.QAJSON, out.VideoPath); saved != nil {
			out.QAJSON = fresh.QAJSON
			return *saved, nil
		}
	}
	path, err := s.storagePath(out.VideoPath)
	if err != nil {
		return clipHearing{}, err
	}
	listener := s.listener()
	words, seconds, err := listenToClip(s, ctx, path, "", hint)
	if err != nil {
		return clipHearing{}, err
	}
	hearing := clipHearing{Video: out.VideoPath, Words: words, Listener: listener, Model: providers.OpenAISpeechModel, Seconds: round3(seconds)}
	if hearing.Words == nil {
		hearing.Words = []providers.SpokenWord{}
	}
	entry := db.CostEntry{Ref: "listen:" + out.ID, Category: "voice", Provider: "openai", Model: providers.OpenAISpeechModel, Label: "Listening to a clip", Quantity: seconds, Unit: "seconds", Amount: seconds / 60 * providers.OpenAISpeechDollarsPerMin, Basis: costs.Rate}
	if listener == "gemini" {
		hearing.Model = providers.GeminiTranscribeModel
		entry.Provider, entry.Model, entry.Amount = "gemini", providers.GeminiTranscribeModel, seconds/60*providers.GeminiTranscribeDollarsPerMin
	}
	if job.ProjectID != nil {
		entry.ProjectID = *job.ProjectID
	}
	if job.SceneID != nil {
		entry.SceneID = *job.SceneID
	}
	_ = s.database.RecordCost(entry)
	if err = s.database.SetRenderOutputNote(out.ID, "speech_timing", voiceJSON(hearing)); err != nil {
		slog.Warn("what was heard in the clip was not saved", "output", out.ID, "reason", err.Error())
	} else if fresh, e := s.database.GetRenderOutput(out.ID); e == nil {
		out.QAJSON = fresh.QAJSON
	}
	hearing.Fresh = true
	return hearing, nil
}

// listenCost is what listening to seconds of a clip costs with the chosen listener.
func (s *Server) listenCost(seconds float64) float64 {
	if s.listener() == "gemini" {
		return seconds / 60 * providers.GeminiTranscribeDollarsPerMin
	}
	return seconds / 60 * providers.OpenAISpeechDollarsPerMin
}

// Caption timing when a line is heard.
const (
	// captionLead shows a caption a moment before the first word; captionHold keeps it
	// after the last, and captionMinimum is the shortest a caption is shown.
	captionLead, captionHold, captionMinimum = 0.08, 0.3, 1.0
	// heardWordingKept is the similarity from which a caption keeps the script's own
	// wording; below it the caption shows what was actually said.
	heardWordingKept = 0.8
)

// heardLines moves each caption of a clip to when its line is actually spoken. Lines
// that were not heard keep their estimated time, kept clear of their neighbours.
func (s *Server) heardLines(ctx context.Context, out *models.RenderOutput, job *models.RenderJob, lines []timedLine, length float64) []timedLine {
	if !nativeSound(job) || len(lines) == 0 {
		return lines // silent clips get their voice (and its times) from Voice from script
	}
	texts := make([]string, len(lines))
	for i, line := range lines {
		texts[i] = line.Text
	}
	hearing, err := s.clipTranscript(ctx, out, job, strings.Join(texts, " "))
	if err != nil {
		if err != errNoListener {
			slog.Warn("captions keep their estimated times: the clip could not be listened to", "output", out.ID, "reason", err.Error())
		}
		return lines
	}
	return placeHeardLines(lines, hearing.Words, length)
}

// placeHeardLines times each line by the words heard for it. A line heard close to
// its script keeps the script's wording; one where something else was said shows what
// was said, with the script kept in Scripted.
func placeHeardLines(lines []timedLine, words []providers.SpokenWord, length float64) []timedLine {
	timed := append([]timedLine{}, lines...)
	if len(words) == 0 {
		return timed
	}
	texts := make([]string, len(lines))
	for i, line := range lines {
		texts[i] = line.Text
	}
	matches, _ := alignHeard(texts, words)
	for i, m := range matches {
		if !m.heard() {
			continue
		}
		heard := joinHeard(words[m.First : m.Last+1])
		spokenStart, spokenEnd := words[m.First].Start, words[m.Last].End
		start := max(0, spokenStart-captionLead)
		end := max(spokenEnd+captionHold, start+captionMinimum)
		if length > 0 {
			end = min(end, length)
		}
		timed[i].Start, timed[i].End, timed[i].Timing = start, end, timingHeard
		timed[i].SpokenStart, timed[i].SpokenEnd = round3(spokenStart), round3(spokenEnd)
		timed[i].Heard, timed[i].Similarity = heard, speechSimilarity(lines[i].Text, heard)
		if timed[i].Similarity < heardWordingKept && strings.TrimSpace(heard) != "" {
			timed[i].Scripted, timed[i].Text = lines[i].Text, heard
		}
	}
	for i := 0; i+1 < len(timed); i++ {
		if timed[i].End > timed[i+1].Start && timed[i+1].Start > timed[i].Start {
			timed[i].End = timed[i+1].Start
		}
	}
	for i := range timed {
		timed[i].Start, timed[i].End = round3(timed[i].Start), round3(timed[i].End)
	}
	return timed
}
