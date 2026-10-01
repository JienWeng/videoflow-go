package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleGetAppSettings(w http.ResponseWriter, r *http.Request) {
	appSettings := map[string]interface{}{
		"default_aspect_ratio":   "9:16",
		"default_video_provider": s.cfg.DefaultVideoProvider,
		"default_image_provider": "openrouter",
		"default_scene_duration": 5.0,
		"caption_style":          "clean",
		"caption_language":       "auto",
		"whisper_model":          "small",
		"dialogue_language":      "English",
		"image_model":            s.cfg.OpenRouterImageModel,
		"ref_image_model":        s.cfg.AtlasImageModel,
		"video_model":            s.cfg.OpenRouterVideoModel,
	}
	writeJSON(w, http.StatusOK, appSettings)
}

func (s *Server) handlePutAppSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	for k, v := range body {
		b, _ := json.Marshal(v)
		_ = s.database.SetAppSetting(k, string(b))
	}
	s.handleGetAppSettings(w, r)
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents := []map[string]interface{}{
		{"agent": "script_agent", "provider": "openrouter", "model": "openai/gpt-4o-mini"},
		{"agent": "scene_agent", "provider": "openrouter", "model": "openai/gpt-4o-mini"},
		{"agent": "shot_agent", "provider": "openrouter", "model": "openai/gpt-4o-mini"},
		{"agent": "asset_planner", "provider": "openrouter", "model": "openai/gpt-4o-mini"},
		{"agent": "prompt_agent", "provider": "openrouter", "model": "openai/gpt-4o-mini"},
		{"agent": "intent_agent", "provider": "openrouter", "model": "openai/gpt-4o-mini"},
	}
	writeJSON(w, http.StatusOK, agents)
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	providers := []map[string]interface{}{
		{"name": "openrouter", "configured": s.cfg.OpenRouterAPIKey != "", "models": []string{s.cfg.OpenRouterVideoModel, "openai/gpt-4o-mini"}},
		{"name": "atlascloud", "configured": s.cfg.AtlasCloudAPIKey != "", "models": []string{s.cfg.AtlasVideoModel, s.cfg.AtlasImageModel}},
	}
	writeJSON(w, http.StatusOK, providers)
}

func (s *Server) handleConnectionPresets(w http.ResponseWriter, r *http.Request) {
	presets := map[string]interface{}{
		"openrouter": map[string]string{"name": "OpenRouter", "url": "https://openrouter.ai/api/v1"},
		"atlascloud": map[string]string{"name": "AtlasCloud", "url": "https://api.atlascloud.ai/v1"},
	}
	writeJSON(w, http.StatusOK, presets)
}
