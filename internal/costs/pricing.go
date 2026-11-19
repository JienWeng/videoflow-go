// Package costs turns what the app asked each paid service to make into dollars.
// Rates are the services' published list prices; where a service reports the exact
// units it billed (OpenAI image tokens, OpenRouter cost) those are used instead.
package costs

import (
	"math"
	"strings"
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

// Image prices one generated picture. usage may be nil when it was not recorded.
func Image(provider, model string, usage *ImageUsage) (float64, string) {
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

// Voice prices one spoken line by the text sent.
func Voice(provider, model, text string) (float64, string) {
	switch provider {
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
