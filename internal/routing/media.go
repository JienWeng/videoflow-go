package routing

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"videoflow-go/internal/costs"

	"videoflow-go/internal/providers"
)

func ValidProviderURL(provider, raw string) bool {
	switch provider {
	case "openrouter":
		return ValidBaseURL(raw)
	case "openai":
		return strings.TrimRight(raw, "/") == providers.OpenAIURL
	case "elevenlabs":
		return strings.TrimRight(raw, "/") == providers.ElevenURL
	case "fish":
		return strings.TrimRight(raw, "/") == providers.FishURL
	case "fal":
		return strings.TrimRight(raw, "/") == providers.FalURL
	case "byteplus":
		return strings.TrimRight(raw, "/") == providers.BytePlusURL
	}
	if host := costs.HostByName(provider); host != nil && providers.VideoHost(provider) {
		return strings.TrimRight(raw, "/") == host.URL
	}
	return false
}
func (r Resolver) mediaCredentials(name string) (key, endpoint string, fromDB bool, err error) {
	switch name {
	case "openai":
		key = r.Config.OpenAIAPIKey
		endpoint = strings.TrimRight(r.Config.OpenAIBaseURL, "/")
		if endpoint == "" {
			endpoint = providers.OpenAIURL
		}
		u, e := url.Parse(endpoint)
		if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return "", "", false, fmt.Errorf("OPENAI_BASE_URL must be an HTTPS API base URL")
		}

	case "elevenlabs":
		key = r.Config.ElevenAPIKey
		endpoint = r.Config.ElevenBaseURL
		if endpoint == "" {
			endpoint = providers.ElevenURL
		}
	case "fish":
		key = r.Config.FishAPIKey
		endpoint = r.Config.FishBaseURL
		if endpoint == "" {
			endpoint = providers.FishURL
		}
	case "fal":
		key = r.Config.FalAPIKey
		endpoint = r.Config.FalBaseURL
		if endpoint == "" {
			endpoint = providers.FalURL
		}
	case "byteplus":
		key = r.Config.BytePlusAPIKey
		endpoint = r.Config.BytePlusBaseURL
		if endpoint == "" {
			endpoint = providers.BytePlusURL
		}
	default:
		host := costs.HostByName(name)
		if host == nil || !providers.VideoHost(name) {
			return "", "", false, fmt.Errorf("unsupported media provider %s", name)
		}
		key, endpoint = os.Getenv(host.EnvKey), host.URL
	}
	stored, base, found, e := r.DB.GetProviderSecret(name)
	if e != nil {
		return "", "", false, e
	}
	if base != nil && *base != "" {
		if !ValidProviderURL(name, *base) {
			return "", "", false, fmt.Errorf("invalid %s API endpoint", name)
		}
		if name != "openai" || r.Config.OpenAIBaseURL == "" {
			endpoint = *base
		}
	}
	if found && stored != "" {
		key = stored
		fromDB = true
	}
	if key == "" {
		return "", endpoint, fromDB, fmt.Errorf("%s API key is not configured; save it in Settings", name)
	}
	return key, endpoint, fromDB, nil
}

type MediaChoice struct {
	Provider string
	Model    string
}

func (r Resolver) MediaSelections() (map[string]MediaChoice, error) {
	values, err := r.DB.AppSettingsSnapshot()
	if err != nil {
		return nil, err
	}
	setting := func(key, fallback string) string {
		var value string
		if json.Unmarshal([]byte(values[key]), &value) == nil && strings.TrimSpace(value) != "" {
			return value
		}
		return fallback
	}
	selections := map[string]MediaChoice{}
	storedOpenAI, _, _, err := r.DB.GetProviderSecret("openai")
	if err != nil {
		return nil, err
	}
	openAIConfigured := r.Config.OpenAIAPIKey != "" || storedOpenAI != ""
	for _, kind := range []string{"image", "reference", "video"} {
		providerKey := "default_image_provider"
		modelKey := "image_model"
		if kind == "video" {
			providerKey = "default_video_provider"
			modelKey = "video_model"
		} else if kind == "reference" {
			modelKey = "ref_image_model"
		}
		if kind != "video" && openAIConfigured {
			selections[kind] = MediaChoice{"openai", r.DefaultMediaModel("openai", kind)}
			continue
		}
		provider := setting(providerKey, "fal")
		selections[kind] = MediaChoice{provider, setting(modelKey, r.DefaultMediaModel(provider, kind))}
		if kind == "video" && setting("video_routing", "manual") == "cheapest" {
			if offer, ok := r.cheapestVideo(setting("video_family", costs.Families[0].ID), setting("default_resolution", "720p"), values); ok {
				selections[kind] = MediaChoice{offer.Provider, offer.Model}
			}
		}
	}
	return selections, nil
}

// cheapestVideo picks the lowest-priced connected service that sells the family and can
// start a clip from a storyboard picture.
func (r Resolver) cheapestVideo(family, resolution string, settings map[string]string) (costs.Offer, bool) {
	for _, offer := range costs.Offers(family, resolution, "16:9", costs.LiveFromSettings(settings)) {
		if offer.Frames && r.Connected(offer.Provider) {
			return offer, true
		}
	}
	return costs.Offer{}, false
}

func (r Resolver) MediaSelection(kind string) (provider, model string) {
	selections, err := r.MediaSelections()
	if err != nil {
		return "fal", r.DefaultMediaModel("fal", kind)
	}
	choice := selections[kind]
	return choice.Provider, choice.Model
}
func (r Resolver) DefaultMediaModel(provider, kind string) string {
	switch provider {
	case "openai":
		if r.Config.OpenAIImageModel != "" {
			return r.Config.OpenAIImageModel
		}
		return providers.OpenAIImageModel
	case "fal":
		if kind == "video" {
			return providers.FalVideoModel
		}
		return providers.FalImageModel
	case "byteplus":
		if kind == "video" {
			return providers.BytePlusVideoModel
		}
		return ""
	default:
		if host := costs.HostByName(provider); host != nil && providers.VideoHost(provider) {
			if kind == "video" {
				return host.Models[costs.Families[0].ID]
			}
			return ""
		}
		if kind == "video" {
			return r.Config.OpenRouterVideoModel
		}
		return r.Config.OpenRouterImageModel
	}
}

// Media binds polling to the job's persisted endpoint, not today's selection.
func (r Resolver) Media(provider, model, endpoint string) (providers.MediaClient, string, error) {
	key, configured, _, err := r.Credentials(provider)
	if err != nil {
		return nil, "", err
	}
	if endpoint == "" {
		endpoint = configured
	}
	if endpoint != configured && !ValidProviderURL(provider, endpoint) {
		return nil, "", fmt.Errorf("invalid persisted provider endpoint")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, "", fmt.Errorf("invalid provider endpoint")
	}
	switch provider {
	case "openai":
		return providers.NewOpenAIImages(key, endpoint, model), endpoint, nil
	case "fal":
		return providers.NewFalClientAt(key, endpoint, model), endpoint, nil
	case "byteplus":
		return providers.NewBytePlusClientAt(key, endpoint), endpoint, nil
	default:
		if providers.VideoHost(provider) {
			return providers.NewHostClient(provider, key, endpoint), endpoint, nil
		}
		return providers.NewOpenRouterClientAt(key, endpoint), endpoint, nil
	}
}
func (r Resolver) SelectedMedia(kind string) (providers.MediaClient, string, string, error) {
	provider, model := r.MediaSelection(kind)
	client, _, err := r.Media(provider, model, "")
	return client, provider, model, err
}
func MediaPreferenceKey(provider, kind string) string {
	return "media." + provider + "." + kind + "_model"
}
