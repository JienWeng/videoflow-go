// Package costs turns what the app asked each paid service to make into dollars.
// Rates are the services' published list prices; where a service reports the exact
// units it billed (OpenAI image tokens, OpenRouter cost) those are used instead.
package costs

import (
	"math"
	"strings"
	"time"
)

// How sure an amount is.
const (
	Metered  = "metered"  // the service reported the units or cost it billed
	Rate     = "rate"     // published rate × what was made
	Estimate = "estimate" // typical price; the exact units were not recorded
	Included = "included" // covered by a subscription, no per-use charge
	Unpriced = "unpriced" // no known price for this service or model
)

// Seedance prices by tokens: width × height × seconds × 24 / 1024.
var seedanceSizes = map[string]map[string][2]int{
	"480p":  {"16:9": {864, 480}, "9:16": {480, 864}, "1:1": {640, 640}, "4:3": {752, 560}, "3:4": {560, 752}, "21:9": {992, 432}},
	"720p":  {"16:9": {1280, 720}, "9:16": {720, 1280}, "1:1": {960, 960}, "4:3": {1112, 834}, "3:4": {834, 1112}, "21:9": {1470, 630}},
	"1080p": {"16:9": {1920, 1080}, "9:16": {1080, 1920}, "1:1": {1440, 1440}, "4:3": {1664, 1248}, "3:4": {1248, 1664}, "21:9": {2206, 946}},
}

func seedanceTokens(resolution, aspect string, seconds float64) float64 {
	sizes, ok := seedanceSizes[resolution]
	if !ok {
		sizes = seedanceSizes["720p"]
	}
	size, ok := sizes[aspect]
	if !ok {
		size = sizes["16:9"]
	}
	return float64(size[0]*size[1]) * seconds * 24 / 1024
}

// Video prices one generated clip. live holds prices read from the services themselves.
func Video(provider, model, resolution, aspect string, seconds float64, live map[string]float64) (amount float64, basis string) {
	if provider == "local" {
		return 0, Included
	}
	if family := FamilyOf(provider, model); family != "" {
		if price, ok := HostPrice(provider, family, resolution, aspect, live); ok {
			return round(price * seconds), Rate
		}
	}
	return 0, Unpriced
}

// ImageUsage is the token count OpenAI returns with a generated image.
type ImageUsage struct {
	InputTokens  float64 `json:"input_tokens"`
	OutputTokens float64 `json:"output_tokens"`
	Details      struct {
		Text  float64 `json:"text_tokens"`
		Image float64 `json:"image_tokens"`
	} `json:"input_tokens_details"`
}

// GeminiImagePrices is Google's paid-tier list price per picture at the default 1K
// size (VideoFlow does not ask for larger ones), read from
// https://aistudio.google.com/docs/pricing on 23 Nov 2026. Thinking drafts are not charged.
var GeminiImagePrices = map[string]float64{
	"gemini-3-pro-image":     0.134,
	"gemini-3.1-flash-image": 0.067,
	"gemini-2.5-flash-image": 0.039,
}

// Image prices one generated picture. usage may be nil when it was not recorded.
func Image(provider, model string, usage *ImageUsage) (float64, string) {
	if provider == "gemini" {
		if price, ok := GeminiImagePrices[model]; ok {
			return price, Rate
		}
		return 0, Unpriced
	}
	switch {
	case provider == "codex" || provider == "local":
		return 0, Included
	case provider == "openai" && strings.HasPrefix(model, "gpt-image"):
		if usage != nil && usage.OutputTokens > 0 {
			text, image := usage.Details.Text, usage.Details.Image
			if text+image == 0 {
				text = usage.InputTokens
			}
			return round((text*5 + image*8 + usage.OutputTokens*30) / 1e6), Metered
		}
		// Typical high-quality widescreen picture; real cost varies with detail.
		return 0.15, Estimate
	}
	return 0, Unpriced
}

// GeminiSpeechPrices is Google's Standard-tier TTS price in dollars per million tokens:
// text in, audio out, at "25 tokens per second of audio". Read from
// https://aistudio.google.com/docs/pricing on 23 Nov 2026; both double on 1 Jan 2027.
var GeminiSpeechPrices = map[string][2]float64{
	"gemini-3.8-flash-tts":      {0.50, 9.00},
	"gemini-3.8-flash-lite-tts": {0.50, 6.00},
}

// geminiSpeechRise is when Google's announced TTS prices take effect.
var geminiSpeechRise = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

// Voice prices one spoken line by the text sent. seconds is how long the line came out
// (0 when unknown); at is when it was made.
func Voice(provider, model, text string, seconds float64, at time.Time) (float64, string) {
	switch provider {
	case "gemini":
		rates, ok := GeminiSpeechPrices[model]
		if !ok {
			return 0, Unpriced
		}
		if !at.Before(geminiSpeechRise) {
			rates[0], rates[1] = rates[0]*2, rates[1]*2
		}
		basis := Rate
		if seconds <= 0 {
			// About 15 characters are spoken a second.
			seconds, basis = float64(len([]rune(text)))/15, Estimate
		}
		// About 4 characters make a text token.
		return round((float64(len([]rune(text)))/4*rates[0] + seconds*25*rates[1]) / 1e6), basis
	case "fish":
		if strings.HasSuffix(model, "-free") {
			return 0, Included
		}
		return round(float64(len(text)) * 15 / 1e6), Rate // $15 per million UTF-8 bytes
	case "elevenlabs":
		return 0, Included // drawn from the monthly character allowance
	}
	return 0, Unpriced
}

func round(v float64) float64 { return math.Round(v*10000) / 10000 }
