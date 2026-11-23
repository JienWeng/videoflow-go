package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// Google Gemini speech (TTS) through the Interactions API, on the Google Gemini key.
// Checked against https://aistudio.google.com/docs/speech-generation, /docs/models and
// /docs/deprecations on 23 Nov 2026. The 2.5 and 3.1 preview TTS models shut down on
// 18 Dec 2026 and are not offered. A unary answer is WAV: "RIFF header (24 kHz, mono,
// 16-bit PCM)". Voices are Google's 30 prebuilt ones; the language is detected from the text.

// GeminiSpeechModels are the offered TTS models; the first is the default.
var GeminiSpeechModels = []SpeechModel{
	{ID: "gemini-3.8-flash-tts", Name: "Gemini 3.8 Flash TTS", Default: true},
	{ID: "gemini-3.8-flash-lite-tts", Name: "Gemini 3.8 Flash-Lite TTS"},
}

// GeminiVoices are the prebuilt voices with Google's one-word description of each.
var GeminiVoices = []SpeechVoice{}

func init() {
	for _, v := range [][2]string{
		{"Zephyr", "Bright"}, {"Puck", "Upbeat"}, {"Charon", "Informative"}, {"Kore", "Firm"}, {"Fenrir", "Excitable"},
		{"Leda", "Youthful"}, {"Orus", "Firm"}, {"Aoede", "Breezy"}, {"Callirrhoe", "Easy-going"}, {"Autonoe", "Bright"},
		{"Enceladus", "Breathy"}, {"Iapetus", "Clear"}, {"Umbriel", "Easy-going"}, {"Algieba", "Smooth"}, {"Despina", "Smooth"},
		{"Erinome", "Clear"}, {"Algenib", "Gravelly"}, {"Rasalgethi", "Informative"}, {"Laomedeia", "Upbeat"}, {"Achernar", "Soft"},
		{"Alnilam", "Firm"}, {"Schedar", "Even"}, {"Gacrux", "Mature"}, {"Pulcherrima", "Forward"}, {"Achird", "Friendly"},
		{"Zubenelgenubi", "Casual"}, {"Vindemiatrix", "Gentle"}, {"Sadachbia", "Lively"}, {"Sadaltager", "Knowledgeable"}, {"Sulafat", "Warm"},
	} {
		GeminiVoices = append(GeminiVoices, SpeechVoice{ID: v[0], Name: v[0], Description: v[1], Labels: map[string]string{"style": strings.ToLower(v[1])}})
	}
}

func geminiSpeechModel(id string) bool {
	for _, m := range GeminiSpeechModels {
		if m.ID == id {
			return true
		}
	}
	return false
}

func geminiVoice(id string) bool {
	for _, v := range GeminiVoices {
		if v.ID == id {
			return true
		}
	}
	return false
}

// geminiSpeechVoices filters the prebuilt voices by a search word.
func geminiSpeechVoices(search string) SpeechVoices {
	out := SpeechVoices{Voices: []SpeechVoice{}}
	search = strings.ToLower(strings.TrimSpace(search))
	for _, v := range GeminiVoices {
		if search == "" || strings.Contains(strings.ToLower(v.Name+" "+v.Description), search) {
			out.Voices = append(out.Voices, v)
		}
	}
	return out
}

// GeminiSpeech speaks text with a prebuilt voice and returns WAV audio. It is one paid
// request and is never retried here.
func GeminiSpeech(ctx context.Context, key, base, model, voice, text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	if !geminiSpeechModel(model) || !geminiVoice(voice) || text == "" || utf8.RuneCountInString(text) > 5000 {
		return nil, fmt.Errorf("Google Gemini speech needs a Gemini TTS model, one of its prebuilt voices and 1–5000 characters")
	}
	c := newMediaHTTP(key, base, "Google Gemini", "")
	c.header = "x-goog-api-key"
	c.errorLimit = geminiErrorBodyLimit
	c.limit = 32 << 20
	c.client.Timeout = 2 * time.Minute
	body := map[string]any{
		"model":             model,
		"input":             []map[string]any{{"type": "user_input", "content": []map[string]string{{"type": "text", "text": text}}}},
		"response_format":   map[string]string{"type": "audio"},
		"generation_config": map[string]any{"speech_config": []map[string]string{{"voice": voice}}},
	}
	raw, err := c.request(ctx, http.MethodPost, "/interactions", body)
	if err != nil {
		return nil, geminiHTTPError(err)
	}
	var answer geminiInteraction
	if json.Unmarshal(raw, &answer) != nil {
		return nil, fmt.Errorf("Google Gemini returned an unreadable speech answer")
	}
	encoded := ""
	for _, step := range answer.Steps {
		if step.Type != "model_output" {
			continue
		}
		for _, content := range step.Content {
			if content.Type == "audio" && content.Data != "" {
				encoded = content.Data
			}
		}
	}
	if encoded == "" {
		reason := answer.Status
		if text := answer.text(); text != "" {
			reason += ": " + text
		}
		return nil, fmt.Errorf("Google Gemini returned no speech (%s)", strings.TrimSpace(reason))
	}
	audio, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(audio) < 44 {
		return nil, fmt.Errorf("Google Gemini returned damaged speech audio")
	}
	if kind := http.DetectContentType(audio); kind != "audio/wave" {
		return nil, fmt.Errorf("Google Gemini returned speech in an unexpected format (%s)", kind)
	}
	return audio, nil
}
