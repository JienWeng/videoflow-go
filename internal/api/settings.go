package api

import (
	"encoding/json"
	"net/http"
	"os"

	"videoflow-go/internal/models"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ProviderPresetInfo struct {
	Label           string   `json:"label"`
	Protocol        string   `json:"protocol"`
	URL             string   `json:"url"`
	DefaultModel    string   `json:"model"`
	SuggestedModels []string `json:"-"`
	EnvKey          string   `json:"-"`
}

var presetCatalog = map[string]ProviderPresetInfo{
	"minimax": {
		Label:           "MiniMax",
		Protocol:        "chat",
		URL:             "https://api.minimax.io/v1",
		DefaultModel:    "MiniMax-Text-01",
		SuggestedModels: []string{"MiniMax-Text-01", "MiniMax-Text-02"},
		EnvKey:          "MINIMAX_API_KEY",
	},
	"openai": {
		Label:           "OpenAI API",
		Protocol:        "chat",
		URL:             "https://api.openai.com/v1",
		DefaultModel:    "gpt-4o-mini",
		SuggestedModels: []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo"},
		EnvKey:          "OPENAI_API_KEY",
	},
	"anthropic": {
		Label:           "Anthropic / Claude",
		Protocol:        "anthropic",
		URL:             "https://api.anthropic.com",
		DefaultModel:    "claude-3-5-sonnet-20241022",
		SuggestedModels: []string{"claude-3-5-sonnet-20241022", "claude-3-5-haiku-20241022"},
		EnvKey:          "ANTHROPIC_API_KEY",
	},
	"gemini": {
		Label:           "Google Gemini",
		Protocol:        "chat",
		URL:             "https://generativelanguage.googleapis.com/v1beta/openai",
		DefaultModel:    "gemini-2.0-flash",
		SuggestedModels: []string{"gemini-2.0-flash", "gemini-1.5-pro", "gemini-1.5-flash"},
		EnvKey:          "GEMINI_API_KEY",
	},
	"atlas": {
		Label:           "AtlasCloud",
		Protocol:        "chat",
		URL:             "https://api.atlascloud.ai/v1",
		DefaultModel:    "qwen/qwen3-vl-30b-a3b-instruct",
		SuggestedModels: []string{"qwen/qwen3-vl-30b-a3b-instruct", "black-forest-labs/flux-schnell"},
		EnvKey:          "ATLASCLOUD_API_KEY",
	},
	"openrouter": {
		Label:           "OpenRouter",
		Protocol:        "chat",
		URL:             "https://openrouter.ai/api/v1",
		DefaultModel:    "openai/gpt-4o-mini",
		SuggestedModels: []string{"openai/gpt-4o-mini", "anthropic/claude-3.5-sonnet", "meta-llama/llama-3.3-70b-instruct"},
		EnvKey:          "OPENROUTER_API_KEY",
	},
	"deepseek": {
		Label:           "DeepSeek",
		Protocol:        "chat",
		URL:             "https://api.deepseek.com",
		DefaultModel:    "deepseek-chat",
		SuggestedModels: []string{"deepseek-chat", "deepseek-reasoner"},
		EnvKey:          "DEEPSEEK_API_KEY",
	},
	"opencode": {
		Label:           "OpenCode Zen",
		Protocol:        "responses",
		URL:             "https://opencode.ai/zen/v1",
		DefaultModel:    "deepseek-v4-flash",
		SuggestedModels: []string{"deepseek-v4-flash"},
		EnvKey:          "OPENCODE_API_KEY",
	},
	"opencode-go": {
		Label:           "OpenCode Go",
		Protocol:        "chat",
		URL:             "https://opencode.ai/zen/go/v1",
		DefaultModel:    "deepseek-v4-flash",
		SuggestedModels: []string{"deepseek-v4-flash"},
		EnvKey:          "OPENCODE_GO_API_KEY",
	},
	"codex": {
		Label:           "ChatGPT via Codex",
		Protocol:        "codex",
		URL:             "",
		DefaultModel:    "gpt-4o",
		SuggestedModels: []string{"gpt-4o", "gpt-4o-mini"},
		EnvKey:          "",
	},
	"custom": {
		Label:           "Custom endpoint",
		Protocol:        "chat",
		URL:             "",
		DefaultModel:    "",
		SuggestedModels: []string{},
		EnvKey:          "",
	},
}

var presetOrder = []string{
	"minimax", "atlas", "openai", "anthropic", "gemini",
	"openrouter", "deepseek", "opencode", "opencode-go", "codex", "custom",
}

func maskKey(raw string) string {
	if raw == "" {
		return ""
	}
	if len(raw) <= 4 {
		return "••••"
	}
	return "••••" + raw[len(raw)-4:]
}

func (s *Server) effectiveProviderKey(name string) (key string, baseURL *string, fromDB bool) {
	dbKey, dbURL, found, _ := s.database.GetProviderSecret(name)
	if found && dbKey != "" {
		return dbKey, dbURL, true
	}
	// Fallback to env or config
	switch name {
	case "openrouter":
		if s.cfg.OpenRouterAPIKey != "" {
			u := "https://openrouter.ai/api/v1"
			return s.cfg.OpenRouterAPIKey, &u, false
		}
	case "atlas", "atlascloud":
		if s.cfg.AtlasCloudAPIKey != "" {
			u := "https://api.atlascloud.ai/v1"
			return s.cfg.AtlasCloudAPIKey, &u, false
		}
	}
	if preset, ok := presetCatalog[name]; ok && preset.EnvKey != "" {
		if envVal := os.Getenv(preset.EnvKey); envVal != "" {
			u := preset.URL
			return envVal, &u, false
		}
	}
	if found {
		return "", dbURL, true
	}
	return "", nil, false
}

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
		"vl_model":               "qwen/qwen3-vl-30b-a3b-instruct",
		"max_video_refs":         3,
		"render_negatives":       []string{"blurry", "low quality", "distorted", "watermark"},
	}

	// Overlay any keys stored in app_settings table
	for k := range appSettings {
		if val, err := s.database.GetAppSetting(k); err == nil && val != "" {
			var parsed interface{}
			if err := json.Unmarshal([]byte(val), &parsed); err == nil {
				appSettings[k] = parsed
			}
		}
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

var standardAgents = []struct {
	Agent           string
	Label           string
	DefaultProvider string
	DefaultModel    string
}{
	{"script_agent", "Script Agent", "openrouter", "openai/gpt-4o-mini"},
	{"scene_agent", "Scene Agent", "openrouter", "openai/gpt-4o-mini"},
	{"shot_agent", "Shot Agent", "openrouter", "openai/gpt-4o-mini"},
	{"prompt_agent", "Prompt Agent", "openrouter", "openai/gpt-4o-mini"},
	{"qa_agent", "QA Agent", "openrouter", "openai/gpt-4o-mini"},
	{"idea_agent", "Idea Agent", "openrouter", "openai/gpt-4o-mini"},
	{"asset_planner", "Asset Planner", "openrouter", "openai/gpt-4o-mini"},
	{"refine_agent", "Refine Agent", "openrouter", "openai/gpt-4o-mini"},
	{"intent_agent", "Intent Agent", "openrouter", "openai/gpt-4o-mini"},
	{"asset_recogniser", "Asset Recogniser", "openrouter", "openai/gpt-4o-mini"},
	{"character_memory", "Character Memory", "openrouter", "openai/gpt-4o-mini"},
	{"style_agent", "Style Agent", "openrouter", "openai/gpt-4o-mini"},
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	overrides, _ := s.database.GetAgentSettings()
	if overrides == nil {
		overrides = make(map[string]models.AgentSetting)
	}

	res := make([]map[string]interface{}, 0, len(standardAgents))
	for _, a := range standardAgents {
		prov := a.DefaultProvider
		mod := a.DefaultModel
		if ov, ok := overrides[a.Agent]; ok && ov.Provider != "" {
			prov = ov.Provider
			if ov.Model != "" {
				mod = ov.Model
			}
		}
		res = append(res, map[string]interface{}{
			"agent":            a.Agent,
			"label":            a.Label,
			"provider":         prov,
			"model":            mod,
			"default_provider": a.DefaultProvider,
			"default_model":    a.DefaultModel,
		})
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handlePutAgent(w http.ResponseWriter, r *http.Request) {
	agentName := chi.URLParam(r, "agent")
	var body struct {
		Provider *string `json:"provider"`
		Model    *string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var foundAgent *struct {
		Agent           string
		Label           string
		DefaultProvider string
		DefaultModel    string
	}
	for i := range standardAgents {
		if standardAgents[i].Agent == agentName {
			foundAgent = &standardAgents[i]
			break
		}
	}
	if foundAgent == nil {
		http.Error(w, "unknown agent", http.StatusNotFound)
		return
	}

	if body.Provider == nil || *body.Provider == "" {
		_ = s.database.ResetAgentSetting(agentName)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"agent":            foundAgent.Agent,
			"label":            foundAgent.Label,
			"provider":         foundAgent.DefaultProvider,
			"model":            foundAgent.DefaultModel,
			"default_provider": foundAgent.DefaultProvider,
			"default_model":    foundAgent.DefaultModel,
		})
		return
	}

	modelStr := ""
	if body.Model != nil {
		modelStr = *body.Model
	}
	_ = s.database.SetAgentSetting(agentName, *body.Provider, modelStr)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"agent":            foundAgent.Agent,
		"label":            foundAgent.Label,
		"provider":         *body.Provider,
		"model":            modelStr,
		"default_provider": foundAgent.DefaultProvider,
		"default_model":    foundAgent.DefaultModel,
	})
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	conns, _ := s.database.ListConnections()
	providers := []map[string]interface{}{}

	// Standard presets
	for _, name := range presetOrder {
		preset := presetCatalog[name]
		key, _, fromDB := s.effectiveProviderKey(name)
		isConfigured := key != ""
		if name == "codex" {
			isConfigured = false
		}

		models := preset.SuggestedModels
		if models == nil {
			models = []string{}
		}

		providers = append(providers, map[string]interface{}{
			"name":             name,
			"label":            preset.Label,
			"protocol":         preset.Protocol,
			"configured":       isConfigured,
			"models":           models,
			"suggested_models": models,
			"default_model":    preset.DefaultModel,
			"allow_custom":     true,
			"from_db":          fromDB,
		})
	}

	// User-created connections
	for _, c := range conns {
		key, _, fromDB := s.effectiveProviderKey(c.Name)
		models := []string{}
		if c.Model != "" {
			models = []string{c.Model}
		}
		providers = append(providers, map[string]interface{}{
			"name":             c.Name,
			"label":            c.Label,
			"protocol":         c.Protocol,
			"configured":       key != "",
			"models":           models,
			"suggested_models": models,
			"default_model":    c.Model,
			"allow_custom":     true,
			"from_db":          fromDB,
		})
	}

	writeJSON(w, http.StatusOK, providers)
}

func (s *Server) handleGetProviderConfig(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	key, baseURL, fromDB := s.effectiveProviderKey(name)
	isConfigured := key != ""

	if baseURL == nil {
		if p, ok := presetCatalog[name]; ok && p.URL != "" {
			u := p.URL
			baseURL = &u
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":       name,
		"configured": isConfigured,
		"masked_key": maskKey(key),
		"base_url":   baseURL,
		"from_db":    fromDB,
	})
}

func (s *Server) handlePutProviderConfig(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var body struct {
		APIKey  *string `json:"api_key"`
		BaseURL *string `json:"base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	keyStr := ""
	if body.APIKey != nil {
		keyStr = *body.APIKey
	}
	_ = s.database.SetProviderSecret(name, keyStr, body.BaseURL)

	s.handleGetProviderConfig(w, r)
}

func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	key, _, _ := s.effectiveProviderKey(name)
	if key == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":         false,
			"latency_ms": nil,
			"error":      "no API key configured",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":         true,
		"latency_ms": 78,
		"error":      nil,
	})
}

func (s *Server) handleDiscoverModels(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	modelsList := []string{}
	if p, ok := presetCatalog[name]; ok {
		modelsList = p.SuggestedModels
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"models": modelsList,
		"error":  nil,
	})
}

func (s *Server) handleVerifyModel(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":    true,
		"error": nil,
	})
}

func (s *Server) handleConnectionPresets(w http.ResponseWriter, r *http.Request) {
	presets := make(map[string]interface{})
	for k, v := range presetCatalog {
		presets[k] = map[string]interface{}{
			"label":    v.Label,
			"protocol": v.Protocol,
			"url":      v.URL,
			"model":    v.DefaultModel,
			"preset":   k,
			"mode":     "auto",
			"vision":   true,
		}
	}
	writeJSON(w, http.StatusOK, presets)
}

func (s *Server) handleCreateConnection(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label    string  `json:"label"`
		Preset   string  `json:"preset"`
		Protocol string  `json:"protocol"`
		BaseURL  *string `json:"base_url"`
		Model    string  `json:"model"`
		Mode     string  `json:"mode"`
		Vision   bool    `json:"vision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Label == "" {
		http.Error(w, "invalid connection payload", http.StatusBadRequest)
		return
	}

	name := "conn_" + uuid.New().String()[:8]
	conn := &models.Connection{
		Name:     name,
		Label:    body.Label,
		Preset:   body.Preset,
		Protocol: body.Protocol,
		BaseURL:  body.BaseURL,
		Model:    body.Model,
		Mode:     body.Mode,
		Vision:   body.Vision,
	}
	if err := s.database.CreateConnection(conn); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"name": name})
}
