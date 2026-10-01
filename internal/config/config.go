package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port                  int
	StorageRoot           string
	DatabasePath          string
	WorkerConcurrency     int
	MaxConcurrentPolls    int
	OpenRouterAPIKey      string
	OpenRouterBaseURL     string
	OpenRouterTextModel   string
	OpenRouterVisionModel string
	OpenRouterImageModel  string
	OpenRouterVideoModel  string
}

func Load() *Config {
	cfg := &Config{
		Port:                  8000,
		StorageRoot:           "storage",
		DatabasePath:          "db.sqlite",
		WorkerConcurrency:     3,
		MaxConcurrentPolls:    5,
		OpenRouterBaseURL:     "https://openrouter.ai/api/v1",
		OpenRouterTextModel:   "openai/gpt-4o-mini",
		OpenRouterVisionModel: "qwen/qwen3-vl-30b-a3b-instruct",
		OpenRouterImageModel:  "openai/gpt-image-2",
		OpenRouterVideoModel:  "google/veo-3.1-lite",
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
	for name, target := range map[string]*string{"OPENROUTER_TEXT_MODEL": &cfg.OpenRouterTextModel, "OPENROUTER_VISION_MODEL": &cfg.OpenRouterVisionModel, "OPENROUTER_IMAGE_MODEL": &cfg.OpenRouterImageModel, "OPENROUTER_VIDEO_MODEL": &cfg.OpenRouterVideoModel} {
		if value := os.Getenv(name); value != "" {
			*target = value
		}
	}
	return cfg
}

func (c *Config) MissingKeys() []string {
	var missing []string
	if c.OpenRouterAPIKey == "" {
		missing = append(missing, "OPENROUTER_API_KEY")
	}
	return missing
}
