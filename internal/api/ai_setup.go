package api

import (
	"fmt"
	"net/http"
	"strings"

	"videoflow-go/internal/costs"
	"videoflow-go/internal/providers"
)

// AI setup: one answer to "which AI does each job right now, and is it ready?".
// The choices themselves are changed through the routes that already own them.

type aiChoice struct {
	Service string `json:"service"` // who does the job, in plain words
	Model   string `json:"model"`
	Ready   bool   `json:"ready"`
	Note    string `json:"note"` // why this one, or what is missing
}

func (s *Server) registerAISetupRoute() {
	s.router.Get("/ai/setup", s.handleAISetup)
}

func (s *Server) handleAISetup(w http.ResponseWriter, r *http.Request) {
	s.refreshKeyChecks()
	ctx, routes := r.Context(), s.routes()
	connected := routes.Connected
	label := func(provider string) string {
		if host := costs.HostByName(provider); host != nil {
			return host.Label
		}
		return serviceLabel(provider)
	}

	// Assistant: the conversation selected in this project.
	engines := map[string]bool{}
	for _, name := range []string{"claude", "codex"} {
		_, err := s.directorRunner.Available(name)
		engines[name] = err == nil && !s.authManager().Status(ctx, name).SignedOut
	}
	assistant := map[string]any{"ready": false, "engines": engines}
	if session, err := s.directorSession(ctx); err == nil {
		assistant["session_id"], assistant["engine"], assistant["title"] = session.ID, session.Engine, session.Title
		assistant["model"], assistant["effort"], assistant["ready"] = session.Model, session.Effort, engines[session.Engine]
	}

	// Pictures: Codex draws them itself when the conversation runs on Codex; otherwise a paid service does.
	pictures := aiChoice{}
	provider, model := routes.MediaSelection("image")
	if s.preferCodexImages(ctx) {
		pictures = aiChoice{Service: "Codex", Model: "built-in picture maker", Ready: true, Note: "Used because this conversation runs on Codex. No charge per picture."}
	} else {
		pictures = aiChoice{Service: label(provider), Model: model, Ready: connected(provider)}
		if provider == "openai" {
			pictures.Note = "Used whenever an OpenAI key is saved, unless Google Gemini is chosen. About $0.15 per picture."
		}
		if price, ok := costs.GeminiImagePrices[model]; ok && provider == "gemini" {
			pictures.Note = fmt.Sprintf("About $%.3f per picture.", price)
		}
		if problem := s.serviceProblem(provider); problem != "" {
			pictures.Note = problem
		} else if !pictures.Ready {
			pictures.Note = "Add a key for " + pictures.Service + " in AI connections."
		}
	}

	// The services pictures can be sent to. A service chosen here is used, whichever keys are saved.
	pictureService := map[string]any{"provider": provider, "model": model}
	choices := []string{"openai", "fal", "openrouter", "gemini"}
	options := []map[string]any{}
	for _, name := range choices {
		options = append(options, map[string]any{"id": name, "label": label(name), "ready": connected(name)})
	}
	pictureService["options"] = options
	if provider == "gemini" {
		models := []map[string]any{}
		for _, m := range providers.GeminiImageModels {
			entry := map[string]any{"id": m.ID, "label": m.Label}
			if price, ok := costs.GeminiImagePrices[m.ID]; ok {
				entry["per_picture"] = price
			}
			models = append(models, entry)
		}
		pictureService["models"] = models
	}

	// Video: one service, picked by hand or by price.
	provider, model = routes.MediaSelection("video")
	video := map[string]any{"service": label(provider), "provider": provider, "model": model, "ready": connected(provider), "routing": routes.Setting("video_routing", "manual"), "family": costs.FamilyOf(provider, model)}
	if check := s.keyCheckView(provider); check != nil && check.State == providers.KeyRejected {
		video["key_rejected"], video["note"] = true, check.Message
	} else if check != nil && check.State == providers.KeyNoCredit {
		video["no_credit"], video["note"] = true, check.Message
	}
	settings, _ := s.database.AppSettingsSnapshot()
	if price, ok := costs.HostPrice(provider, costs.FamilyOf(provider, model), "720p", "16:9", costs.LiveFromSettings(settings)); ok {
		video["per_second"] = price
	}
	for _, f := range costs.Families {
		if f.ID == video["family"] {
			video["family_label"] = f.Label
		}
	}

	// Voice: the service that speaks narration and dialogue.
	primary := s.primaryVoiceProvider()
	voices := []map[string]any{}
	for _, name := range []string{"fish", "elevenlabs", "gemini"} {
		voices = append(voices, map[string]any{"id": name, "label": label(name), "ready": connected(name)})
	}
	voice := map[string]any{"provider": primary, "service": label(primary), "ready": connected(primary), "options": voices}

	// Looking at pictures goes through OpenRouter; listening to clips to time captions uses
	// OpenAI or Google Gemini.
	openrouter := connected("openrouter")
	note := ""
	if !openrouter {
		note = "Add an OpenRouter key in AI connections."
	}
	listener := s.listener()
	listenKey, _, _, _ := routes.Credentials(listener)
	listening := aiChoice{Service: label(listener), Model: providers.OpenAISpeechModel, Ready: strings.TrimSpace(listenKey) != "" && connected(listener)}
	if listener == "gemini" {
		listening.Model = providers.GeminiTranscribeModel
	}
	if !listening.Ready {
		listening.Note = "Add an OpenAI or Google Gemini key in AI connections. Without one, captions keep their estimated times."
	}
	listenOptions := []map[string]any{}
	for _, name := range []string{"openai", "gemini"} {
		listenOptions = append(listenOptions, map[string]any{"id": name, "label": label(name), "ready": connected(name)})
	}
	writeJSON(w, 200, map[string]any{
		"assistant": assistant, "pictures": pictures, "picture_service": pictureService, "video": video, "voice": voice,
		"vision":    aiChoice{Service: "OpenRouter", Model: routes.Setting("vl_model", s.cfg.OpenRouterVisionModel), Ready: openrouter, Note: note},
		"listening": listening, "listening_service": map[string]any{"provider": listener, "options": listenOptions},
	})
}
