package costs

import (
	"path"
	"sort"
	"strconv"
	"strings"
)

// A video model (a "family") is sold by several services under different names and
// prices. This table is what lets the app compare them and pick the cheapest.

// Family is one video model the user can choose, whoever hosts it.
type Family struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

var Families = []Family{
	{"seedance-2.0", "Seedance 2.0"}, {"seedance-2.0-fast", "Seedance 2.0 Fast"}, {"seedance-2.0-mini", "Seedance 2.0 Mini"},
	{"seedance-2.5", "Seedance 2.5"}, {"seedance-2.5-flash", "Seedance 2.5 Flash"},
	{"gemini-omni-flash", "Gemini Omni Flash (Google)"},
}

// Host is one service that sells video generation.
type Host struct {
	Name   string
	Label  string
	URL    string
	EnvKey string
	// Models maps a family to the service's own model name.
	Models map[string]string
	// PerSecond is the list price in dollars per second: family → resolution → price.
	PerSecond map[string]map[string]float64
	// TokenRate is dollars per 1,000 Seedance tokens, for services that bill by tokens.
	TokenRate map[string]float64
	// References is how many reference pictures one clip may be given instead of a first
	// frame (Seedance's omni reference mode); 0 when the service does not take them.
	References int
	// Voices is how many voice recordings a reference-mode clip may also be given.
	Voices int
	// Checked says where the price came from.
	Checked string
	// DefaultResolution is what the service makes when no size is asked for ("auto");
	// empty when that is not known, which leaves such clips unpriced.
	DefaultResolution string
}

// Prices were read from each service's public pricing page on 19 Nov 2026, or from the
// comparison supplied by the user where a page gave no per-second figure.
var Hosts = []Host{
	{Name: "argolink", Label: "ArgoLink", URL: "https://argolink.io/v1", EnvKey: "ARGOLINK_API_KEY", References: 9, Voices: 3, Checked: "argolink.io model page",
		Models: map[string]string{"seedance-2.0": "seedance-2.0", "seedance-2.0-fast": "seedance-2.0-fast", "seedance-2.0-mini": "seedance-2.0-mini", "seedance-2.5": "seedance-2.5", "seedance-2.5-flash": "seedance-2.5-flash"},
		PerSecond: map[string]map[string]float64{"seedance-2.0": {"480p": 0.0598, "720p": 0.129, "1080p": 0.318}, "seedance-2.0-fast": {"480p": 0.0478, "720p": 0.103},
			"seedance-2.0-mini": {"480p": 0.0299, "720p": 0.0643}, "seedance-2.5": {"480p": 0.0874, "720p": 0.196, "1080p": 0.483}, "seedance-2.5-flash": {"480p": 0.0572, "720p": 0.129}}},
	{Name: "byteplus", Label: "BytePlus (official)", URL: "https://ark.ap-southeast.bytepluses.com/api/v3", EnvKey: "BYTEPLUS_API_KEY", Checked: "official token rate, as passed through by OpenRouter",
		Models:    map[string]string{"seedance-2.0": "dreamina-seedance-2-0-260128", "seedance-2.0-fast": "dreamina-seedance-2-0-fast-260128"},
		TokenRate: map[string]float64{"seedance-2.0": 0.007, "seedance-2.0-fast": 0.0056}},
	{Name: "openrouter", Label: "OpenRouter", URL: "https://openrouter.ai/api/v1", EnvKey: "OPENROUTER_API_KEY", Checked: "openrouter.ai live price feed",
		Models: map[string]string{"seedance-2.0": "bytedance/seedance-2.0", "seedance-2.0-fast": "bytedance/seedance-2.0-fast", "seedance-2.0-mini": "bytedance/seedance-2.0-mini",
			"seedance-2.5": "bytedance/seedance-2.5", "seedance-2.5-flash": "bytedance/seedance-2.5-flash"},
		TokenRate: map[string]float64{"seedance-2.0": 0.007, "seedance-2.0-fast": 0.0056, "seedance-2.0-mini": 0.0035, "seedance-2.5": 0.0107, "seedance-2.5-flash": 0.007}},
	{Name: "fal", Label: "fal.ai", URL: "https://queue.fal.run", EnvKey: "FAL_KEY", Checked: "fal.ai price list",
		Models: map[string]string{"seedance-2.0": "bytedance/seedance-2.0/image-to-video", "seedance-2.0-fast": "bytedance/seedance-2.0/fast/image-to-video",
			"seedance-2.0-mini": "bytedance/seedance-2.0/mini/image-to-video", "seedance-2.5": "bytedance/seedance-2.5/image-to-video"},
		TokenRate: map[string]float64{"seedance-2.0": 0.014, "seedance-2.0-fast": 0.0112, "seedance-2.0-mini": 0.007, "seedance-2.5": 0.0214}},
	// Google bills Omni video as output tokens at $12.00 per million, "5,792 tokens per second
	// of 720p video"; 360p is 1,931, 1080p 8,688 and 4K 17,376 tokens per second
	// (https://aistudio.google.com/docs/pricing, checked 23 Nov 2026). The prompt and pictures
	// add input tokens at $1.50 per million, well under a cent per clip, which is left out.
	{Name: "gemini", Label: "Google Gemini", URL: "https://generativelanguage.googleapis.com/v1beta", EnvKey: "GEMINI_API_KEY", Checked: "Google Gemini API pricing page, 23 Nov 2026", DefaultResolution: "720p",
		Models:    map[string]string{"gemini-omni-flash": "gemini-omni-1.2-flash"},
		PerSecond: map[string]map[string]float64{"gemini-omni-flash": {"360p": 1931 * 12 / 1e6, "720p": 5792 * 12 / 1e6, "1080p": 8688 * 12 / 1e6, "4k": 17376 * 12 / 1e6}}},
}

// Retired labels the services the app used to send video to. Old video jobs and cost
// entries still carry these names, and a saved choice may still name one.
var Retired = map[string]string{"replicate": "Replicate", "kie": "Kie.ai", "wavespeed": "WaveSpeed", "runway": "Runway", "loopling": "Loopling"}

func HostByName(name string) *Host {
	for i := range Hosts {
		if Hosts[i].Name == name {
			return &Hosts[i]
		}
	}
	return nil
}

// FamilyOf names the family a service's model belongs to, or "".
func FamilyOf(provider, model string) string {
	h := HostByName(provider)
	if h == nil {
		return ""
	}
	for family, id := range h.Models {
		if id == model {
			return family
		}
	}
	// fal names one endpoint per kind of input under a model's path, such as
	// bytedance/seedance-2.0/fast/text-to-video beside .../fast/image-to-video.
	if provider == "fal" {
		for family, id := range h.Models {
			if path.Dir(id) == path.Dir(model) {
				return family
			}
		}
	}
	return ""
}

// LiveFromSettings collects the prices last read from the services themselves. Keys are
// "provider:family" for a token rate (dollars per 1,000 tokens) and
// "provider:family:resolution" for dollars per second.
func LiveFromSettings(settings map[string]string) map[string]float64 {
	live := map[string]float64{}
	for key, raw := range settings {
		for _, prefix := range []string{"video_rate_", "video_price_"} {
			if name, found := strings.CutPrefix(key, prefix); found {
				if value, err := strconv.ParseFloat(raw, 64); err == nil && value > 0 {
					live[name] = value
				}
			}
		}
	}
	return live
}

// HostPrice is the dollars per second a service charges for a family at a resolution.
// live holds prices read from the services themselves; they replace the built-in ones.
func HostPrice(provider, family, resolution, aspect string, live map[string]float64) (float64, bool) {
	h := HostByName(provider)
	if h == nil || family == "" {
		return 0, false
	}
	if resolution == "" {
		resolution = "720p"
	}
	if resolution == "auto" && h.DefaultResolution != "" {
		resolution = h.DefaultResolution
	}
	if rate, ok := h.TokenRate[family]; ok {
		if fresh := live[provider+":"+family]; fresh > 0 {
			rate = fresh
		}
		if _, known := seedanceSizes[resolution]; !known {
			return 0, false
		}
		return seedanceTokens(resolution, aspect, 1) / 1000 * rate, true
	}
	if fresh := live[provider+":"+family+":"+resolution]; fresh > 0 {
		return fresh, true
	}
	price, ok := h.PerSecond[family][resolution]
	return price, ok
}

// Offer is one service's price for the chosen model.
type Offer struct {
	Provider  string  `json:"provider"`
	Label     string  `json:"label"`
	Model     string  `json:"model"`
	PerSecond float64 `json:"per_second"`
	Checked   string  `json:"checked"`
}

// Offers lists every service that sells the family at the resolution, cheapest first.
func Offers(family, resolution, aspect string, live map[string]float64) []Offer {
	out := []Offer{}
	for _, h := range Hosts {
		model, sold := h.Models[family]
		if !sold {
			continue
		}
		price, ok := HostPrice(h.Name, family, resolution, aspect, live)
		if !ok {
			continue
		}
		out = append(out, Offer{Provider: h.Name, Label: h.Label, Model: model, PerSecond: round(price), Checked: h.Checked})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PerSecond < out[j].PerSecond })
	return out
}
