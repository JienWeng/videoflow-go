package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Google Gemini listening: Gemini 3.5 Transcribe with word-level timestamps through the
// Interactions API, on the Google Gemini key. Checked against
// https://aistudio.google.com/docs/transcribe, /docs/audio and /docs/pricing on 23 Nov 2026.
// Words come back as content annotations of type "word_info" with "text",
// "start_offset" and "end_offset" ("0.100s"). Google notes that word timestamps "may
// degrade overall transcription accuracy"; only the times are used here, the script
// supplies the words.
const GeminiTranscribeModel = "gemini-3.5-transcribe"

// GeminiTranscribeDollarsPerMin is Google's per-minute estimate: "$0.003/min" of audio in
// plus "$0.002/min" of text out.
const GeminiTranscribeDollarsPerMin = 0.005

// GeminiTranscribeWords listens to WAV audio and returns each word with when it is
// spoken. language is a BCP-47 code such as "en", or empty to let Google detect it.
func GeminiTranscribeWords(ctx context.Context, key, base string, wav []byte, language string) ([]SpokenWord, error) {
	// Inline data keeps the whole request under Google's 20 MB limit.
	if len(wav) == 0 || len(wav) > 14<<20 {
		return nil, fmt.Errorf("audio must be between 1 byte and 14 MiB")
	}
	if http.DetectContentType(wav) != "audio/wave" {
		return nil, fmt.Errorf("Google Gemini listening needs WAV audio")
	}
	c := newMediaHTTP(key, base, "Google Gemini", "")
	c.header = "x-goog-api-key"
	c.client.Timeout = 2 * time.Minute
	config := map[string]any{"mode": map[string]any{"type": "verbatim", "timestamp_granularities": []string{"word"}}}
	if language != "" {
		config["language_codes"] = []string{language}
	}
	body := map[string]any{
		"model":             GeminiTranscribeModel,
		"input":             []map[string]string{{"type": "audio", "data": base64.StdEncoding.EncodeToString(wav), "mime_type": "audio/wav"}},
		"generation_config": map[string]any{"transcription_config": config},
	}
	raw, err := c.request(ctx, http.MethodPost, "/interactions", body)
	if err != nil {
		return nil, geminiHTTPError(err)
	}
	var answer struct {
		Status string `json:"status"`
		Steps  []struct {
			Type    string `json:"type"`
			Content []struct {
				Annotations []struct {
					Type  string `json:"type"`
					Text  string `json:"text"`
					Start string `json:"start_offset"`
					End   string `json:"end_offset"`
				} `json:"annotations"`
			} `json:"content"`
		} `json:"steps"`
	}
	if json.Unmarshal(raw, &answer) != nil {
		return nil, fmt.Errorf("Google Gemini returned an unreadable transcript")
	}
	if answer.Status != "completed" {
		return nil, fmt.Errorf("Google Gemini did not finish listening (%s)", answer.Status)
	}
	words := []SpokenWord{}
	for _, step := range answer.Steps {
		if step.Type != "model_output" {
			continue
		}
		for _, content := range step.Content {
			for _, a := range content.Annotations {
				start, okStart := geminiOffset(a.Start)
				end, okEnd := geminiOffset(a.End)
				if a.Type == "word_info" && strings.TrimSpace(a.Text) != "" && okStart && okEnd && end >= start {
					words = append(words, SpokenWord{Word: a.Text, Start: start, End: end})
				}
			}
		}
	}
	return words, nil
}

// geminiOffset reads a duration such as "0.100s".
func geminiOffset(text string) (float64, bool) {
	seconds, found := strings.CutSuffix(strings.TrimSpace(text), "s")
	if !found {
		return 0, false
	}
	value, err := strconv.ParseFloat(seconds, 64)
	return value, err == nil && value >= 0
}
