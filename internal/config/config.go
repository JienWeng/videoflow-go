package config

import (
	"os"
	"strconv"
)

type Config struct {
	DisableCodexImages    bool
	OpenAIAPIKey          string
	OpenAIBaseURL         string
	OpenAIImageModel      string
	CodexExecutable       string
	ClaudeExecutable      string
	ElevenAPIKey          string
	ElevenBaseURL         string
	FishSchemaURL         string
	FishAPIKey            string
	FishBaseURL           string
	FalAPIKey             string
	FalBaseURL            string
	BytePlusAPIKey        string
	BytePlusBaseURL       string
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
		DisableCodexImages: os.Getenv("VIDEOFLOW_DISABLE_CODEX_IMAGES") == "1",
		OpenAIAPIKey:       os.Getenv("OPENAI_API_KEY"), OpenAIBaseURL: os.Getenv("OPENAI_BASE_URL"), OpenAIImageModel: os.Getenv("OPENAI_IMAGE_MODEL"),
		CodexExecutable: os.Getenv("VIDEOFLOW_CODEX_BIN"), ClaudeExecutable: os.Getenv("VIDEOFLOW_CLAUDE_BIN"),
		ElevenAPIKey: os.Getenv("ELEVENLABS_API_KEY"), ElevenBaseURL: "https://api.elevenlabs.io",
		FishAPIKey: os.Getenv("FISH_API_KEY"), FishBaseURL: "https://api.fish.audio",
		FalAPIKey: os.Getenv("FAL_KEY"), FalBaseURL: "https://queue.fal.run", BytePlusAPIKey: os.Getenv("BYTEPLUS_API_KEY"), BytePlusBaseURL: "https://ark.ap-southeast.bytepluses.com/api/v3",
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
