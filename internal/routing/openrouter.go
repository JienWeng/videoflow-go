// Package routing resolves current OpenRouter settings without caching secrets.
package routing

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"videoflow-go/internal/config"
	"videoflow-go/internal/costs"
	"videoflow-go/internal/db"
	"videoflow-go/internal/providers"
)

type Resolver struct {
	Config *config.Config
	DB     *db.DB
}

func ValidBaseURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "openrouter.ai" && strings.TrimRight(u.Path, "/") == "/api/v1" && u.RawQuery == "" && u.Fragment == "" && u.User == nil
}

func (r Resolver) Credentials(name string) (key, endpoint string, fromDB bool, err error) {
	// Old video jobs may still name a service the app no longer uses.
	if label := costs.Retired[name]; label != "" {
		return "", "", false, fmt.Errorf("%s is no longer available in VideoFlow; choose another video service", label)
	}
	if name == "openai" || name == "gemini" || name == "fal" || name == "byteplus" || name == "fish" || name == "elevenlabs" || providers.VideoHost(name) {
		return r.mediaCredentials(name)
	}
	endpoint = r.Config.OpenRouterBaseURL
	if endpoint == "" {
		endpoint = providers.OpenRouterURL
	}
	if name == "" {
		name = "openrouter"
	}
	if name != "openrouter" {
		connections, e := r.DB.ListConnections()
		if e != nil {
			return "", "", false, e
		}
		found := false
		for _, c := range connections {
			if c.Name == name && c.Preset == "openrouter" && c.Protocol == "chat" {
				found = true
				if c.BaseURL != nil && *c.BaseURL != "" {
					if !ValidBaseURL(*c.BaseURL) {
						return "", "", false, fmt.Errorf("saved connection must use https://openrouter.ai/api/v1")
					}
					endpoint = *c.BaseURL
				}
				break
			}
		}
		if !found {
			return "", "", false, fmt.Errorf("connection %q is not an OpenRouter connection; select OpenRouter in settings", name)
		}
	}
	stored, base, found, e := r.DB.GetProviderSecret(name)
	if e != nil {
		return "", "", false, e
	}
	if base != nil && *base != "" {
		if !ValidBaseURL(*base) {
			return "", "", false, fmt.Errorf("connection must use https://openrouter.ai/api/v1")
		}
		endpoint = *base
	}
	if found && stored != "" {
		return stored, endpoint, true, nil
	}
	if name != "openrouter" {
		return "", endpoint, found, fmt.Errorf("OpenRouter connection %s needs an API key", name)
	}
	if r.Config.OpenRouterAPIKey != "" {
		return r.Config.OpenRouterAPIKey, endpoint, false, nil
	}
	return "", endpoint, found, fmt.Errorf("OpenRouter API key is not configured; save it in Settings")
}

func (r Resolver) Client(name string) (*providers.OpenRouterClient, error) {
	if name == "fal" || name == "byteplus" || name == "fish" || name == "elevenlabs" {
		return nil, fmt.Errorf("%s is a media provider; text and vision use OpenRouter", name)
	}
	key, endpoint, _, err := r.Credentials(name)
	if err != nil {
		return nil, err
	}
	return providers.NewOpenRouterClientAt(key, endpoint), nil
}

func (r Resolver) Setting(key, fallback string) string {
	raw, err := r.DB.GetAppSetting(key)
	if err == nil && raw != "" {
		var value string
		if json.Unmarshal([]byte(raw), &value) == nil && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return fallback
}

func (r Resolver) MediaModel(kind string) string { _, model := r.MediaSelection(kind); return model }

func (r Resolver) NumberSetting(key string, fallback float64) float64 {
	raw, err := r.DB.GetAppSetting(key)
	var value float64
	if err == nil && json.Unmarshal([]byte(raw), &value) == nil && value > 0 {
		return value
	}
	return fallback
}
func (r Resolver) StringListSetting(key string, fallback []string) []string {
	raw, err := r.DB.GetAppSetting(key)
	var values []string
	if err == nil && json.Unmarshal([]byte(raw), &values) == nil {
		return values
	}
	return fallback
}
