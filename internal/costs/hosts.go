package costs

import (
	"sort"
	"strings"
)

// A video model (a "family") is sold by several services under different names and
// prices. This table is what lets the app compare them and pick the cheapest.

// Family is one video model the user can choose, whoever hosts it.
type Family struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

var Families = []Family{{"seedance-2.0", "Seedance 2.0"}, {"seedance-2.0-fast", "Seedance 2.0 Fast"}}

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
	// Frames is true when a clip can start from a picture (needed for storyboard scenes).
	Frames bool
	// Checked says where the price came from.
	Checked string
}

// Prices were read from each service's public pricing page on 19 Nov 2026, or from the
// comparison supplied by the user where a page gave no per-second figure.
var Hosts = []Host{
	{Name: "argolink", Label: "ArgoLink", URL: "https://argolink.io/v1", EnvKey: "ARGOLINK_API_KEY", Frames: true, Checked: "argolink.io model page",
		Models:    map[string]string{"seedance-2.0": "seedance-2.0", "seedance-2.0-fast": "seedance-2.0-fast"},
		PerSecond: map[string]map[string]float64{"seedance-2.0": {"480p": 0.0598, "720p": 0.129, "1080p": 0.318}, "seedance-2.0-fast": {"480p": 0.042}}},
	{Name: "loopling", Label: "Loopling", URL: "https://loopling.ai/v1", EnvKey: "LOOPLING_API_KEY", Frames: false, Checked: "loopling.ai pricing",
		Models:    map[string]string{"seedance-2.0": "seedance-2.0", "seedance-2.0-fast": "seedance-2.0-fast"},
		PerSecond: map[string]map[string]float64{"seedance-2.0": {"480p": 0.0604, "720p": 0.13, "1080p": 0.3218}, "seedance-2.0-fast": {"480p": 0.0483, "720p": 0.104}}},
	{Name: "byteplus", Label: "BytePlus (official)", URL: "https://ark.ap-southeast.bytepluses.com/api/v3", EnvKey: "BYTEPLUS_API_KEY", Frames: true, Checked: "official token rate, as passed through by OpenRouter",
		Models:    map[string]string{"seedance-2.0": "dreamina-seedance-2-0-260128", "seedance-2.0-fast": "dreamina-seedance-2-0-fast-260128"},
		TokenRate: map[string]float64{"seedance-2.0": 0.007, "seedance-2.0-fast": 0.0056}},
	{Name: "openrouter", Label: "OpenRouter", URL: "https://openrouter.ai/api/v1", EnvKey: "OPENROUTER_API_KEY", Frames: true, Checked: "openrouter.ai live price feed",
		Models:    map[string]string{"seedance-2.0": "bytedance/seedance-2.0", "seedance-2.0-fast": "bytedance/seedance-2.0-fast"},
		TokenRate: map[string]float64{"seedance-2.0": 0.007, "seedance-2.0-fast": 0.0056}},
	{Name: "replicate", Label: "Replicate", URL: "https://api.replicate.com/v1", EnvKey: "REPLICATE_API_TOKEN", Frames: true, Checked: "your comparison",
		Models:    map[string]string{"seedance-2.0": "bytedance/seedance-2.0", "seedance-2.0-fast": "bytedance/seedance-2.0-fast"},
		PerSecond: map[string]map[string]float64{"seedance-2.0": {"720p": 0.18}}},
	{Name: "kie", Label: "Kie.ai", URL: "https://api.kie.ai/api", EnvKey: "KIE_API_KEY", Frames: true, Checked: "your comparison",
		Models:    map[string]string{"seedance-2.0": "bytedance/seedance-2", "seedance-2.0-fast": "bytedance/seedance-2-fast"},
		PerSecond: map[string]map[string]float64{"seedance-2.0": {"720p": 0.205}}},
	{Name: "wavespeed", Label: "WaveSpeed", URL: "https://api.wavespeed.ai/api/v3", EnvKey: "WAVESPEED_API_KEY", Frames: true, Checked: "wavespeed.ai model page",
		Models:    map[string]string{"seedance-2.0": "bytedance/seedance-2.0", "seedance-2.0-fast": "bytedance/seedance-2.0-fast"},
		PerSecond: map[string]map[string]float64{"seedance-2.0": {"480p": 0.108, "720p": 0.216, "1080p": 0.54}}},
	{Name: "runway", Label: "Runway API", URL: "https://api.dev.runwayml.com/v1", EnvKey: "RUNWAYML_API_SECRET", Frames: true, Checked: "dev.runwayml.com pricing",
		Models:    map[string]string{"seedance-2.0": "seedance2", "seedance-2.0-fast": "seedance2_fast"},
		PerSecond: map[string]map[string]float64{"seedance-2.0": {"480p": 0.14, "720p": 0.30, "1080p": 0.40}, "seedance-2.0-fast": {"480p": 0.11, "720p": 0.24}}},
	{Name: "higgsfield", Label: "Higgsfield API", URL: "https://api.higgsfield.ai", EnvKey: "HIGGSFIELD_API_KEY", Frames: true, Checked: "your comparison (approximate)",
		Models:    map[string]string{"seedance-2.0": "bytedance/seedance-2.0"},
		PerSecond: map[string]map[string]float64{"seedance-2.0": {"720p": 0.302}}},
	{Name: "fal", Label: "fal.ai", URL: "https://queue.fal.run", EnvKey: "FAL_KEY", Frames: true, Checked: "fal.ai price list",
		Models:    map[string]string{"seedance-2.0": "bytedance/seedance-2.0/image-to-video", "seedance-2.0-fast": "bytedance/seedance-2.0/fast/image-to-video"},
		TokenRate: map[string]float64{"seedance-2.0": 0.014, "seedance-2.0-fast": 0.0112}},
}

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
	if provider == "fal" && strings.Contains(model, "seedance-2.0") {
		if strings.Contains(model, "/fast/") {
			return "seedance-2.0-fast"
		}
		return "seedance-2.0"
	}
	return ""
}

// HostPrice is the dollars per second a service charges for a family at a resolution.
// override, when above zero, replaces the token rate (fal publishes its rate live).
func HostPrice(provider, family, resolution, aspect string, override float64) (float64, bool) {
	h := HostByName(provider)
	if h == nil || family == "" {
		return 0, false
	}
	if resolution == "" {
		resolution = "720p"
	}
	if rate, ok := h.TokenRate[family]; ok {
		if override > 0 {
			rate = override
		}
		if _, known := seedanceSizes[resolution]; !known {
			return 0, false
		}
		return seedanceTokens(resolution, aspect, 1) / 1000 * rate, true
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
	Frames    bool    `json:"frames"`
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
		price, ok := HostPrice(h.Name, family, resolution, aspect, live[h.Name+":"+family])
		if !ok {
			continue
		}
		out = append(out, Offer{Provider: h.Name, Label: h.Label, Model: model, PerSecond: round(price), Frames: h.Frames, Checked: h.Checked})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PerSecond < out[j].PerSecond })
	return out
}
