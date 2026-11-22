package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"

	"videoflow-go/internal/agents"
	"videoflow-go/internal/config"
	"videoflow-go/internal/db"
	"videoflow-go/internal/director"
	"videoflow-go/internal/events"
	"videoflow-go/internal/jobs"
	"videoflow-go/internal/media"
	"videoflow-go/internal/providers"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

type Server struct {
	directorRunner  *director.Runner
	agentAuth       *director.AuthManager
	agentAuthOnce   sync.Once
	directorCancels sync.Map
	cfg             *config.Config
	database        *db.DB
	broker          *events.Broker
	workers         *jobs.WorkerPool
	agents          *agents.AgentEngine
	openrouter      *providers.OpenRouterClient
	media           *media.MediaEngine
	router          *chi.Mux
	ctx             context.Context
	cancel          context.CancelFunc
	tasks           sync.WaitGroup
	activeRequests  sync.Map
	recoveryMu      sync.Mutex
	referenceLocks  sync.Map
	paidRequests    chan struct{}
	operationSlots  chan struct{}
}

func NewServer(
	cfg *config.Config,
	database *db.DB,
	broker *events.Broker,
	workers *jobs.WorkerPool,
	agentsEngine *agents.AgentEngine,
	openrouter *providers.OpenRouterClient,
	mediaEngine *media.MediaEngine,
) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		directorRunner: &director.Runner{Codex: cfg.CodexExecutable, Claude: cfg.ClaudeExecutable},
		ctx:            ctx, cancel: cancel, paidRequests: make(chan struct{}, 3), operationSlots: make(chan struct{}, 3),
		cfg:        cfg,
		database:   database,
		broker:     broker,
		workers:    workers,
		agents:     agentsEngine,
		openrouter: openrouter,
		media:      mediaEngine,
		router:     chi.NewRouter(),
	}
	s.agents.SetLanguageResolver(func() string { return s.routes().Setting("dialogue_language", "English") })
	s.agents.SetStructuredRunner(func(ctx context.Context, name, system, prompt string, target any) error {
		return s.localTextJSON(ctx, name, system, prompt, target)
	})
	s.workers.SetSubmissionValidator(s.validateFrozenVideo)
	s.setupRoutes()
	return s
}

func (s *Server) Close() { s.cancel(); s.tasks.Wait() }

func (s *Server) Router() http.Handler {
	return s.router
}

func (s *Server) Database() *db.DB {
	return s.database
}

func (s *Server) setupRoutes() {
	s.useMiddleware()
	s.registerDirectorRoutes()
	s.registerCompositionRoutes()
	s.RegisterVoiceCaptionRoutes()
	s.registerServiceRoutes()
	s.registerProjectRoutes()
	s.registerSceneRoutes()
	s.registerAssetRoutes()
	s.registerRenderRoutes()
	s.registerStyleRoutes()
	s.registerSettingsRoutes()
	s.registerPipelineRoutes()
}

// useMiddleware installs the middleware every route runs through. chi requires it to be
// installed before any route.
func (s *Server) useMiddleware() {
	s.router.Use(middleware.RequestID)
	s.router.Use(s.bindDirectorRequest)
	s.router.Use(s.requestReplay)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	// Every request is cut off after five minutes except the event stream, which stays
	// open for as long as the page does.
	limit := middleware.Timeout(5 * time.Minute)
	s.router.Use(func(next http.Handler) http.Handler {
		limited := limit(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/events" {
				next.ServeHTTP(w, r)
				return
			}
			limited.ServeHTTP(w, r)
		})
	})

	// CORS matching SvelteKit frontend origins
	s.router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://localhost:4173", "http://127.0.0.1:4173"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "Idempotency-Key"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
}

// registerServiceRoutes serves stored files, request recovery, health and the event stream.
func (s *Server) registerServiceRoutes() {
	// Static storage server
	_ = os.MkdirAll(s.cfg.StorageRoot, 0755)
	fileServer := http.FileServer(http.Dir(s.cfg.StorageRoot))
	s.router.Handle("/storage/*", http.StripPrefix("/storage/", fileServer))

	// Recovery does not submit or retry any paid job.
	s.router.Get("/generation-requests", s.handlePendingRequests)
	s.router.Post("/generation-requests/{key}/acknowledge", s.handleAcknowledgeRequest)

	s.router.Get("/health", s.handleHealth)

	// Server-Sent Events (SSE)
	s.router.Get("/events", s.handleEvents)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]interface{}{
		"status":         "ok",
		"media_ready":    s.media.Available(),
		"missing_keys":   s.missingKeys(),
		"image_provider": func() string { p, _ := s.routes().MediaSelection("image"); return p }(),
		"video_provider": func() string { p, _ := s.routes().MediaSelection("video"); return p }(),
		"text_model":     s.cfg.OpenRouterTextModel,
		"image_model":    s.routes().MediaModel("image"),
		"video_model":    s.routes().MediaModel("video"),
		"server":         "videoflow-go-v1",
	}
	writeJSON(w, http.StatusOK, resp)
}

// missingKeys names the API keys the selected text, image and video services still need.
func (s *Server) missingKeys() []string {
	missing := []string{}
	if _, err := s.routes().Client("openrouter"); err != nil {
		missing = append(missing, "OPENROUTER_API_KEY")
	}
	seen := map[string]bool{}
	for _, kind := range []string{"image", "video"} {
		provider, _ := s.routes().MediaSelection(kind)
		if provider == "openrouter" || seen[provider] {
			continue
		}
		seen[provider] = true
		if _, _, _, err := s.routes().Credentials(provider); err != nil {
			if provider == "fal" {
				missing = append(missing, "FAL_KEY")
			} else if provider == "byteplus" {
				missing = append(missing, "BYTEPLUS_API_KEY")
			}
		}
	}
	return missing
}

// registerProjectRoutes serves projects and their characters.
func (s *Server) registerProjectRoutes() {
	// Projects
	s.router.Get("/projects", s.handleListProjects)
	s.router.Post("/projects", s.handleCreateProject)
	s.router.Get("/projects/active", s.handleGetActiveProject)
	s.router.Post("/projects/{id}/activate", s.handleActivateProject)
	s.router.Patch("/projects/{id}", s.handleUpdateProject)
	s.router.Delete("/projects/{id}", s.handleDeleteProject)
	s.router.Get("/projects/{id}/export", s.handleExportProject)
	s.router.Post("/projects/import", s.handleImportProject)

	// Characters
	s.router.Get("/characters", s.handleListCharacters)
	s.router.Post("/characters", s.handleCreateCharacter)
	s.router.Get("/characters/{id}", s.handleGetCharacter)
	s.router.Patch("/characters/{id}", s.handleUpdateCharacter)
	s.router.Delete("/characters/{id}", s.handleDeleteCharacter)
	s.router.Post("/characters/{id}/bible", s.handleCharacterBible)
	s.router.Post("/characters/{id}/reference-sheets", s.handleCharacterRefSheets)
}

// registerSceneRoutes serves scenes, shots, revisions, ideas and scripts.
func (s *Server) registerSceneRoutes() {
	// Scenes, Shots, Scripts & Ideas
	s.router.Get("/scenes", s.handleListScenes)
	s.router.Get("/production/progress", s.handleProjectProgress)
	s.router.Post("/scenes", s.handleCreateScene)
	s.router.Get("/scenes/{id}", s.handleGetScene)
	s.router.Patch("/scenes/{id}", s.handleUpdateScene)
	s.router.Put("/scenes/{id}/style", s.handleSceneStyle)
	s.router.Delete("/scenes/{id}", s.handleDeleteScene)
	s.router.Post("/scenes/{id}/generate", s.handleExpandScene)
	s.router.Post("/scenes/{id}/refine", s.handleRefineScene)
	s.router.Post("/scenes/{id}/render", s.handleRenderScene)
	s.router.Get("/scenes/{id}/production", s.handleProductionPlan)
	s.router.Put("/scenes/{id}/production", s.handleProductionPlan)
	s.router.Post("/scenes/{id}/storyboard/audit", s.handleStoryboardAudit)
	s.router.Post("/scenes/{id}/storyboard", s.handleStoryboard)
	s.router.Get("/scenes/{id}/references", s.handleSceneReferences)
	s.router.Post("/scenes/{id}/references/{asset_id}", s.handleSetSceneReference)
	s.router.Delete("/scenes/{id}/references/{asset_id}", s.handleSetSceneReference)
	s.router.Post("/scenes/{id}/assets/plan", s.handlePlanSceneAssets)
	s.router.Post("/scenes/{id}/assets/generate", s.handleGenerateSceneAssets)
	s.router.Post("/scenes/{id}/shots/generate", s.handleGenerateShots)
	s.router.Post("/scenes/{id}/cast/{character_id}", s.handleCastMemberAdd)
	s.router.Delete("/scenes/{id}/cast/{character_id}", s.handleCastMemberDelete)
	s.router.Get("/scenes/{id}/revisions", s.handleSceneRevisions)
	s.router.Post("/scenes/conversationalize", s.handleConversationalize)

	s.router.Get("/scenes/{id}/shots", s.handleListShots)
	s.router.Post("/scenes/{id}/shots", s.handleCreateShot)
	s.router.Put("/scenes/{id}/shots/order", s.handleReorderShots)
	s.router.Patch("/shots/{id}", s.handleUpdateShot)
	s.router.Delete("/shots/{id}", s.handleDeleteShot)
	s.router.Post("/shots/{id}/refine", s.handleRefineShot)
	s.router.Post("/shots/{id}/assets/{asset_id}", s.handleShotAssetAttach)
	s.router.Delete("/shots/{id}/assets/{asset_id}", s.handleShotAssetDetach)
	s.router.Get("/shots/{id}/revisions", s.handleShotRevisions)
	s.router.Post("/revisions/{id}/revert", s.handleRevertRevision)

	s.router.Post("/ideas/develop", s.handleDevelopIdea)
	s.router.Get("/scripts", s.handleListScripts)
}

// registerAssetRoutes serves the asset library.
func (s *Server) registerAssetRoutes() {
	// Assets
	s.router.Post("/assets/generate", s.handleGenerateImage)
	s.router.Get("/assets", s.handleListAssets)
	s.router.Get("/assets/types", s.handleAssetTypes)
	s.router.Post("/assets/upload", s.handleUploadAsset)
	s.router.Post("/assets/import-link", s.handleImportLinkAsset)
	s.router.Patch("/assets/{id}", s.handleUpdateAssetDetails)
	s.router.Post("/assets/{id}/regenerate", s.handleRegenerateAsset)
	s.router.Get("/assets/{id}/usage", s.handleAssetUsage)
	s.router.Get("/assets/{id}", s.handleGetAsset)
	s.router.Delete("/assets/{id}", s.handleDeleteAsset)
	s.router.Post("/assets/{id}/recognise", s.handleRecogniseAsset)
}

// registerRenderRoutes serves render jobs and their outputs.
func (s *Server) registerRenderRoutes() {
	// Render & Outputs
	s.router.Post("/render/from-shot", s.handleRenderFromShot)
	s.router.Get("/render-jobs", s.handleListRenderJobs)
	s.router.Get("/render-jobs/{id}", s.handleGetRenderJob)
	s.router.Post("/render-jobs/{id}/resubmit", s.handleResubmitJob)
	s.registerRenderRecoveryRoutes()

	s.router.Post("/outputs/{id}/retry", s.handleRetryOutput)
	s.router.Get("/outputs/{id}/download", s.handleDownloadOutput)
}

// registerStyleRoutes serves style presets, preset packs and the project style.
func (s *Server) registerStyleRoutes() {
	// Style Guide
	s.router.Get("/style-presets", s.handlePresetCatalog)
	s.router.Post("/preset-packs/preview", s.handlePresetPack)
	s.router.Post("/preset-packs/import", s.handlePresetPack)
	s.router.Delete("/preset-packs/{id}", s.handleDeletePresetPack)
	s.router.Get("/preset-packs/{id}/export", s.handleExportPresetPack)
	s.router.Get("/style", s.handleGetStyle)
	s.router.Patch("/style", s.handleUpdateStyle)
	s.router.Post("/style/ingest", s.handleIngestStyle)
	s.router.Post("/style/reference", s.handleAnalyzeStyleReference)
}

// registerSettingsRoutes serves app settings, providers and connections.
func (s *Server) registerSettingsRoutes() {
	// Settings
	s.router.Get("/settings/app", s.handleGetAppSettings)
	s.router.Put("/settings/app", s.handlePutAppSettings)
	s.router.Get("/settings/providers", s.handleListProviders)
	s.router.Get("/settings/providers/{name}", s.handleGetProviderConfig)
	s.router.Put("/settings/providers/{name}", s.handlePutProviderConfig)
	s.router.Post("/settings/providers/{name}/test", s.handleTestProvider)
	s.router.Get("/settings/providers/{name}/models", s.handleDiscoverModels)
	s.router.Post("/settings/providers/{name}/verify-model", s.handleVerifyModel)
	s.router.Get("/settings/connection-presets", s.handleConnectionPresets)
	s.router.Post("/settings/connections", s.handleCreateConnection)
}

// registerPipelineRoutes serves chat, the guided video pipeline, operations and the graph.
func (s *Server) registerPipelineRoutes() {
	// Chat Assistant
	s.router.Post("/chat", s.handleChat)

	// Videos (Guided Pipeline)
	s.router.Get("/videos/preflight", s.handleVideoPreflight)
	s.router.Post("/videos/generate", s.handleGenerateVideo)
	s.router.Post("/videos/generate/{id}/continue", s.handleContinueStory)
	s.router.Post("/videos/generate/{id}/assets/exclude", s.handleExcludeStoryAsset)

	// Ops
	s.router.Get("/ops/{id}", s.handleGetOp)
	s.router.Get("/ops", s.handleListOps)

	// Graph Canvas
	s.router.Get("/graph", s.handleGetGraph)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
