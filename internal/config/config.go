package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port                 int
	StorageRoot          string
	DatabasePath         string
	WorkerConcurrency    int
	MaxConcurrentPolls   int
	DefaultVideoProvider string
	OpenRouterAPIKey     string
	AtlasCloudAPIKey     string
	MiniMaxAPIKey        string
	OpenRouterImageModel string
	OpenRouterVideoModel string
	AtlasImageModel      string
	AtlasVideoModel      string
	MiniMaxTextModel     string
}

func Load() *Config {
	cfg := &Config{
		Port:                 8000,
		StorageRoot:          "storage",
		DatabasePath:         "db.sqlite",
		WorkerConcurrency:    3,
		MaxConcurrentPolls:   5,
		DefaultVideoProvider: "openrouter",
		OpenRouterImageModel: "google/imagen-3",
		OpenRouterVideoModel: "minimax/h3-developer/text-to-video",
		AtlasImageModel:      "flux-schnell",
		AtlasVideoModel:      "kling-v1-5",
		MiniMaxTextModel:     "abab6.5s-chat",
	}

	if p := os.Getenv("PORT"); p != "" {
		if val, err := strconv.Atoi(p); err == nil {
			cfg.Port = val
		}
	}
	if s := os.Getenv("STORAGE_ROOT"); s != "" {
		cfg.StorageRoot = s
	}
	if d := os.Getenv("DATABASE_PATH"); d != "" {
		cfg.DatabasePath = d
	}
	if orKey := os.Getenv("OPENROUTER_API_KEY"); orKey != "" {
		cfg.OpenRouterAPIKey = orKey
	}
	if acKey := os.Getenv("ATLASCLOUD_API_KEY"); acKey != "" {
		cfg.AtlasCloudAPIKey = acKey
	}
	if mmKey := os.Getenv("MINIMAX_API_KEY"); mmKey != "" {
		cfg.MiniMaxAPIKey = mmKey
	}
	if dvp := os.Getenv("DEFAULT_VIDEO_PROVIDER"); dvp != "" {
		cfg.DefaultVideoProvider = dvp
	}
	return cfg
}

func (c *Config) MissingKeys() []string {
	var missing []string
	if c.OpenRouterAPIKey == "" {
		missing = append(missing, "OPENROUTER_API_KEY")
	}
	if c.AtlasCloudAPIKey == "" {
		missing = append(missing, "ATLASCLOUD_API_KEY")
	}
	return missing
}
