package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"videoflow-go/internal/costs"
	"videoflow-go/internal/providers"
	"videoflow-go/internal/routing"

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
	"elevenlabs": {Label: "ElevenLabs", Protocol: "speech", URL: providers.ElevenURL, DefaultModel: "auto", EnvKey: "ELEVENLABS_API_KEY"},
	"fish":       {Label: "Fish Audio", Protocol: "speech", URL: providers.FishURL, DefaultModel: "s2.1-pro", SuggestedModels: []string{"s2.1-pro", "s2.1-pro-free", "s2-pro"}, EnvKey: "FISH_API_KEY"},
	"fal":        {Label: "fal.ai", Protocol: "media", URL: providers.FalURL, DefaultModel: providers.FalVideoModel, SuggestedModels: []string{providers.FalVideoModel, providers.FalImageModel}, EnvKey: "FAL_KEY"},
	"openai":     {Label: "OpenAI Images", Protocol: "image", URL: providers.OpenAIURL, DefaultModel: providers.OpenAIImageModel, SuggestedModels: []string{providers.OpenAIImageModel}, EnvKey: "OPENAI_API_KEY"},
	"gemini":     {Label: "Google Gemini", Protocol: "image", URL: providers.GeminiURL, DefaultModel: providers.GeminiImageModel, SuggestedModels: geminiModelIDs(), EnvKey: "GEMINI_API_KEY"},
	"byteplus":   {Label: "BytePlus ModelArk (Seedance)", Protocol: "video", URL: providers.BytePlusURL, DefaultModel: providers.BytePlusVideoModel, SuggestedModels: []string{providers.BytePlusVideoModel}, EnvKey: "BYTEPLUS_API_KEY"},
	"openrouter": {Label: "OpenRouter", Protocol: "chat", URL: providers.OpenRouterURL, DefaultModel: "openai/gpt-4o-mini", SuggestedModels: []string{"openai/gpt-4o-mini", "meta-llama/llama-3.3-70b-instruct"}, EnvKey: "OPENROUTER_API_KEY"},
}

func geminiModelIDs() []string {
	ids := []string{}
	for _, m := range providers.GeminiImageModels {
		ids = append(ids, m.ID)
	}
	return ids
}

func init() {
	added := 0
	for _, host := range costs.Hosts {
		if providers.VideoHost(host.Name) {
			models := []string{}
			for _, family := range costs.Families {
				if id := host.Models[family.ID]; id != "" {
					models = append(models, id)
				}
			}
			presetCatalog[host.Name] = ProviderPresetInfo{Label: host.Label, Protocol: "video", URL: host.URL, DefaultModel: host.Models[costs.Families[0].ID], SuggestedModels: models, EnvKey: host.EnvKey}
			// Video services sit together, right after BytePlus.
			for i, name := range presetOrder {
				if name == "byteplus" {
					at := i + 1 + added
					presetOrder = append(presetOrder[:at], append([]string{host.Name}, presetOrder[at:]...)...)
					added++
					break
				}
			}
		}
	}
}

var presetOrder = []string{"fal", "openai", "gemini", "openrouter", "byteplus", "fish", "elevenlabs"}

func maskKey(raw string) string {
	if raw == "" {
		return ""
	}
	if len(raw) <= 4 {
		return "••••"
	}
	return "••••" + raw[len(raw)-4:]
}

func (s *Server) routes() routing.Resolver { return routing.Resolver{Config: s.cfg, DB: s.database} }
func (s *Server) effectiveProviderKey(name string) (key string, baseURL *string, fromDB bool) {
	key, endpoint, fromDB, _ := s.routes().Credentials(name)
	return key, &endpoint, fromDB
}

func (s *Server) handleGetAppSettings(w http.ResponseWriter, r *http.Request) {
	appSettings := map[string]interface{}{
		"default_aspect_ratio":   "9:16",
		"default_video_provider": "fal",
		"default_image_provider": "fal",
		"default_scene_duration": 5.0,
		"dialogue_language":      "English",
		"image_model":            s.cfg.OpenRouterImageModel,
		"ref_image_model":        s.cfg.OpenRouterImageModel,
		"video_model":            s.cfg.OpenRouterVideoModel,
		"vl_model":               s.cfg.OpenRouterVisionModel,
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

	imageProvider, imageModel := s.routes().MediaSelection("image")
	videoProvider, videoModel := s.routes().MediaSelection("video")
	appSettings["default_image_provider"] = imageProvider
	appSettings["default_video_provider"] = videoProvider
	appSettings["image_model"] = imageModel
	appSettings["ref_image_model"] = s.routes().MediaModel("reference")
	appSettings["video_model"] = videoModel
	prefs := map[string]map[string]string{}
	for _, provider := range []string{"fal", "gemini", "openrouter", "byteplus"} {
		prefs[provider] = map[string]string{}
		for _, kind := range []string{"image", "reference", "video"} {
			prefs[provider][kind] = s.routes().Setting(routing.MediaPreferenceKey(provider, kind), s.routes().DefaultMediaModel(provider, kind))
		}
	}
	appSettings["media_model_preferences"] = prefs
	appSettings["all_text_local"] = true
	writeJSON(w, http.StatusOK, appSettings)
}

func (s *Server) handlePutAppSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.prepareMediaSettings(r.Context(), body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	for key, modality := range map[string]string{"vl_model": "vision"} {
		if value, ok := body[key]; ok {
			model, valid := value.(string)
			if !valid || strings.TrimSpace(model) == "" {
				http.Error(w, "model ID is required", 400)
				return
			}
			client, err := s.routes().Client("openrouter")
			if err == nil {
				_, err = client.Model(r.Context(), model, modality)
			}
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
		}
	}
	if raw, ok := body["default_scene_duration"]; ok {
		value, valid := raw.(float64)
		if !valid || value < 1 || value > 120 || value != float64(int(value)) {
			http.Error(w, "Scene duration must be an integer between 1 and 120 seconds", 400)
			return
		}
	}
	if raw, ok := body["render_negatives"]; ok {
		values, valid := raw.([]any)
		if !valid {
			http.Error(w, "render_negatives must be a list of strings", 400)
			return
		}
		for _, value := range values {
			if _, valid := value.(string); !valid {
				http.Error(w, "render_negatives must contain strings", 400)
				return
			}
		}
	}
	if err := s.database.SetAppSettings(body); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	s.handleGetAppSettings(w, r)
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	s.refreshKeyChecks()
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
			"key_check":        s.keyCheckView(name),
		})
	}

	// User-created connections
	for _, c := range conns {
		if c.Preset != "openrouter" || c.Protocol != "chat" {
			continue
		}
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
		"key_check":  s.keyCheckView(name),
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

	if _, standard := presetCatalog[name]; !standard {
		conns, _ := s.database.ListConnections()
		valid := false
		for _, c := range conns {
			if c.Name == name && c.Preset == "openrouter" {
				valid = true
			}
		}
		if !valid {
			http.Error(w, "Unknown OpenRouter connection", 400)
			return
		}
	}
	if body.BaseURL != nil && *body.BaseURL != "" && !routing.ValidProviderURL(func() string {
		if _, ok := presetCatalog[name]; ok {
			return name
		}
		return "openrouter"
	}(), *body.BaseURL) {
		http.Error(w, "Use the official endpoint for this provider", 400)
		return
	}
	keyStr, _, _, err := s.database.GetProviderSecret(name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if body.APIKey != nil {
		keyStr = strings.TrimSpace(*body.APIKey)
	}
	// A new video service key is asked about before it is kept. If the service cannot be
	// reached the key is still saved, and the screen says it could not be checked.
	var check *providers.KeyCheck
	if body.APIKey != nil && keyStr != "" && providers.VideoHost(name) {
		result := checkVideoKey(r.Context(), name, keyStr, costs.HostByName(name).URL)
		if result.State == providers.KeyRejected {
			http.Error(w, result.Message, 400)
			return
		}
		check = &result
	}
	if err = s.database.SetProviderSecret(name, keyStr, body.BaseURL); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if check != nil {
		_ = s.routes().SaveKeyCheck(name, keyStr, *check)
	}

	s.handleGetProviderConfig(w, r)
}

func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if providers.VideoHost(name) {
		key, endpoint, _, err := s.routes().Credentials(name)
		if err != nil {
			writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error(), "latency_ms": 0, "checked": "key", "credentials_verified": false, "generation_tested": false})
			return
		}
		start := time.Now()
		check := checkVideoKey(r.Context(), name, key, endpoint)
		_ = s.routes().SaveKeyCheck(name, key, check)
		label := costs.HostByName(name).Label
		var problem any
		message := label + " accepted this key. Nothing was made or charged."
		if check.State != providers.KeyAccepted {
			message = check.Message
		}
		usable := check.State != providers.KeyRejected && check.State != providers.KeyNoCredit
		if !usable {
			problem = check.Message
		}
		writeJSON(w, 200, map[string]any{"ok": usable, "error": problem, "latency_ms": time.Since(start).Milliseconds(), "checked": "key", "credentials_verified": check.State == providers.KeyAccepted, "generation_tested": false, "message": message})
		return
	}
	// fal.ai and BytePlus have no free way to ask about the balance: Check again forgets
	// the "no balance left" answer and the next real video decides.
	if check := s.keyCheckView(name); check != nil && check.State == providers.KeyNoCredit && name != "openrouter" {
		s.routes().ForgetNoCredit(name)
		writeJSON(w, 200, map[string]any{"ok": true, "error": nil, "latency_ms": 0, "checked": "key", "credentials_verified": false, "generation_tested": false, "message": costs.HostByName(name).Label + " has no free way to check the balance, so VideoFlow will use it again. The next video shows whether the top-up worked."})
		return
	}
	if name == "openai" || name == "gemini" || name == "fal" || name == "byteplus" || name == "fish" || name == "elevenlabs" {
		_, _, _, err := s.routes().Credentials(name)
		var message any
		if err != nil {
			message = err.Error()
		}
		writeJSON(w, 200, map[string]any{"ok": err == nil, "error": message, "latency_ms": 0, "checked": "configuration", "credentials_verified": false, "generation_tested": false, "message": "Configuration checked; account/model access requires a later generation test."})
		return
	}
	start := time.Now()
	client, err := s.routes().Client(chi.URLParam(r, "name"))
	if err == nil {
		err = client.CheckKey(r.Context())
		s.routes().NoteServiceAnswer(name, err)
	}
	var message any
	if err != nil {
		message = err.Error()
	}
	writeJSON(w, 200, map[string]any{"ok": err == nil, "latency_ms": time.Since(start).Milliseconds(), "error": message})
}
func (s *Server) handleDiscoverModels(w http.ResponseWriter, r *http.Request) {
	modality := r.URL.Query().Get("modality")
	if modality == "" {
		modality = "text"
	}
	name := chi.URLParam(r, "name")
	if r.URL.Query().Get("modality") == "" {
		if name == "fal" || name == "gemini" {
			modality = "image"
		} else if name == "byteplus" || providers.VideoHost(name) {
			modality = "video"
		}
	}
	client, err := s.mediaCatalogClient(name, "")
	var catalog []providers.Model
	if err == nil {
		catalog, err = client.Models(r.Context(), modality)
	}
	ids := []string{}
	for _, model := range catalog {
		ids = append(ids, model.ID)
	}
	var message any
	if err != nil {
		message = err.Error()
	}
	writeJSON(w, 200, map[string]any{"models": ids, "capabilities": catalog, "modality": modality, "error": message})
}
func (s *Server) handleVerifyModel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Model    string `json:"model"`
		Modality string `json:"modality"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.Model) == "" {
		http.Error(w, "model is required", 400)
		return
	}
	if body.Modality == "" {
		body.Modality = "text"
		name := chi.URLParam(r, "name")
		if name == "byteplus" || providers.VideoHost(name) {
			body.Modality = "video"
		} else if name == "gemini" {
			body.Modality = "image"
		} else if name == "fal" {
			body.Modality = "video"
			if strings.HasPrefix(body.Model, providers.FalImageModel) {
				body.Modality = "image"
			}
		}
	}
	name := chi.URLParam(r, "name")
	client, err := s.mediaCatalogClient(name, body.Model)
	if err == nil {
		_, err = client.Model(r.Context(), body.Model, body.Modality)
	}
	if err == nil && (body.Modality == "text" || body.Modality == "vision") {
		text, e := s.routes().Client(name)
		err = e
		if err == nil {
			var result struct {
				OK bool `json:"ok"`
			}
			err = text.StructuredJSON(r.Context(), body.Model, "Return the requested JSON.", "Return {\"ok\":true}", &result)
			if err == nil && !result.OK {
				err = fmt.Errorf("model did not return requested JSON")
			}
		}
	}
	var message any
	if err != nil {
		message = err.Error()
	}
	writeJSON(w, 200, map[string]any{"ok": err == nil, "error": message, "modality": body.Modality, "generation_tested": (body.Modality == "text" || body.Modality == "vision") && err == nil})
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

	if body.Preset != "openrouter" || (body.Protocol != "" && body.Protocol != "chat") {
		http.Error(w, "Connections must use OpenRouter Chat Completions", 400)
		return
	}
	if body.BaseURL != nil && *body.BaseURL != "" && !routing.ValidBaseURL(*body.BaseURL) {
		http.Error(w, "Use https://openrouter.ai/api/v1", 400)
		return
	}
	body.Protocol = "chat"
	if body.Mode != "auto" && body.Mode != "prompt" && body.Mode != "" {
		http.Error(w, "Supported output modes are auto and prompt", 400)
		return
	}
	if body.BaseURL == nil || *body.BaseURL == "" {
		value := providers.OpenRouterURL
		body.BaseURL = &value
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
