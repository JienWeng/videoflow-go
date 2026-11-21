package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"unicode"

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
	key, endpoint, _, err := s.routes().Credentials("openai")
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
	words, err := providers.NewOpenAIImages(key, endpoint, "").TranscribeWords(ctx, audio, language, hint)
	// 16 kHz mono 16-bit audio is 32,000 bytes a second.
	return words, float64(len(audio)) / 32000, err
}

type listenerError string

func (e listenerError) Error() string { return string(e) }

const errNoListener = listenerError("no OpenAI key is saved, so captions keep their estimated times")

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

// heardWords returns the words spoken in a finished clip, listening to it the first
// time and remembering the answer on the clip so it is paid for once.
func (s *Server) heardWords(ctx context.Context, out *models.RenderOutput, job *models.RenderJob, language, hint string) []providers.SpokenWord {
	notes := map[string]json.RawMessage{}
	_ = json.Unmarshal(out.QAJSON, &notes)
	var saved struct {
		Video string                 `json:"video"`
		Words []providers.SpokenWord `json:"words"`
	}
	if json.Unmarshal(notes["speech_timing"], &saved) == nil && saved.Video == out.VideoPath {
		return saved.Words
	}
	path, err := s.storagePath(out.VideoPath)
	if err != nil {
		return nil
	}
	words, seconds, err := listenToClip(s, ctx, path, speechCode(language), hint)
	if err != nil {
		if err != errNoListener {
			slog.Warn("captions keep their estimated times: the clip could not be listened to", "output", out.ID, "reason", err.Error())
		}
		return nil
	}
	saved.Video, saved.Words = out.VideoPath, words
	notes["speech_timing"] = voiceJSON(saved)
	out.QAJSON = voiceJSON(notes)
	if err = s.database.UpdateRenderOutput(out); err != nil {
		slog.Warn("heard caption times were not saved", "output", out.ID, "reason", err.Error())
	}
	entry := db.CostEntry{Ref: "listen:" + out.ID, Category: "voice", Provider: "openai", Model: providers.OpenAISpeechModel, Label: "Caption timing", Quantity: seconds, Unit: "seconds", Amount: seconds / 60 * providers.OpenAISpeechDollarsPerMin, Basis: costs.Rate}
	if job.ProjectID != nil {
		entry.ProjectID = *job.ProjectID
	}
	if job.SceneID != nil {
		entry.SceneID = *job.SceneID
	}
	_ = s.database.RecordCost(entry)
	return words
}

// speechLetters reduces text to the letters and digits that are compared, so
// punctuation, spacing and capitals never matter; Chinese is compared character by
// character, which is also how it is heard.
func speechLetters(text string) []rune {
	out := []rune{}
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out = append(out, r)
		}
	}
	return out
}

// heardSpans finds when each line is spoken. Lines and words are both in order, so the
// whole script is lined up against everything heard at once (the cheapest set of
// insertions, deletions and substitutions); a line counts as heard when most of its
// letters were found, and its time runs from its first heard word to its last.
func heardSpans(lines []string, words []providers.SpokenWord) [][2]float64 {
	spans := make([][2]float64, len(lines))
	for i := range spans {
		spans[i] = [2]float64{-1, -1}
	}
	var script, heard []rune
	var lineOf, wordOf []int
	for i, line := range lines {
		for _, r := range speechLetters(line) {
			script, lineOf = append(script, r), append(lineOf, i)
		}
	}
	for i, word := range words {
		for _, r := range speechLetters(word.Word) {
			heard, wordOf = append(heard, r), append(wordOf, i)
		}
	}
	if len(script) == 0 || len(heard) == 0 || len(script)*len(heard) > 4<<20 {
		return spans
	}
	n, m := len(script), len(heard)
	cost := make([][]int32, n+1)
	for i := range cost {
		cost[i] = make([]int32, m+1)
		cost[i][0] = int32(i)
	}
	for j := 0; j <= m; j++ {
		cost[0][j] = int32(j)
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			best := cost[i-1][j-1]
			if script[i-1] != heard[j-1] {
				best++
			}
			best = min(best, cost[i-1][j]+1, cost[i][j-1]+1)
			cost[i][j] = best
		}
	}
	found := make([]int, len(lines))
	first, last := make([]int, len(lines)), make([]int, len(lines))
	for i := range first {
		first[i], last[i] = -1, -1
	}
	for i, j := n, m; i > 0 && j > 0; {
		switch {
		case script[i-1] == heard[j-1] && cost[i][j] == cost[i-1][j-1]:
			line, word := lineOf[i-1], wordOf[j-1]
			found[line]++
			first[line] = word
			if last[line] < 0 {
				last[line] = word
			}
			i, j = i-1, j-1
		case cost[i][j] == cost[i-1][j-1]+1:
			i, j = i-1, j-1
		case cost[i][j] == cost[i-1][j]+1:
			i--
		default:
			j--
		}
	}
	for i, line := range lines {
		if letters := len(speechLetters(line)); letters > 0 && found[i]*5 >= letters*3 {
			spans[i] = [2]float64{words[first[i]].Start, words[last[i]].End}
		}
	}
	return spans
}

// heardLines moves each caption of a clip to when its line is actually spoken. Lines
// that were not heard keep their estimated time, kept clear of their neighbours.
func (s *Server) heardLines(ctx context.Context, out *models.RenderOutput, job *models.RenderJob, lines []timedLine, length float64) []timedLine {
	var request struct {
		Sound bool `json:"generate_audio"`
	}
	if json.Unmarshal(job.RequestJSON, &request) != nil || !request.Sound || len(lines) == 0 {
		return lines // silent clips get their voice (and its times) from Voice from script
	}
	texts := make([]string, len(lines))
	for i, line := range lines {
		texts[i] = line.Text
	}
	words := s.heardWords(ctx, out, job, lines[0].Language, strings.Join(texts, " "))
	if len(words) == 0 {
		return lines
	}
	spans := heardSpans(texts, words)
	timed := append([]timedLine{}, lines...)
	for i, span := range spans {
		if span[0] < 0 {
			continue
		}
		// Up a moment before the first word and held briefly after the last, long
		// enough to read.
		start := max(0, span[0]-0.08)
		end := max(span[1]+0.3, start+1)
		if length > 0 {
			end = min(end, length)
		}
		timed[i].Start, timed[i].End = start, end
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
