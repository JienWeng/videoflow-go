package routing

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"videoflow-go/internal/providers"
)

func ValidProviderURL(provider, raw string) bool {
	switch provider {
	case "openrouter":
		return ValidBaseURL(raw)
	case "fal":
		return strings.TrimRight(raw, "/") == providers.FalURL
	case "byteplus":
		return strings.TrimRight(raw, "/") == providers.BytePlusURL
	}
	return false
}
func (r Resolver) mediaCredentials(name string) (key, endpoint string, fromDB bool, err error) {
	switch name {
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
		return "", "", false, fmt.Errorf("unsupported media provider %s", name)
	}
	stored, base, found, e := r.DB.GetProviderSecret(name)
	if e != nil {
		return "", "", false, e
	}
	if base != nil && *base != "" {
		if !ValidProviderURL(name, *base) {
			return "", "", false, fmt.Errorf("invalid %s API endpoint", name)
		}
		endpoint = *base
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
	for _, kind := range []string{"image", "reference", "video"} {
		providerKey := "default_image_provider"
		modelKey := "image_model"
		if kind == "video" {
			providerKey = "default_video_provider"
			modelKey = "video_model"
		} else if kind == "reference" {
			modelKey = "ref_image_model"
		}
		provider := setting(providerKey, "fal")
		selections[kind] = MediaChoice{provider, setting(modelKey, r.DefaultMediaModel(provider, kind))}
	}
	return selections, nil
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
	case "fal":
		return providers.NewFalClientAt(key, endpoint, model), endpoint, nil
	case "byteplus":
		return providers.NewBytePlusClientAt(key, endpoint), endpoint, nil
	default:
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
