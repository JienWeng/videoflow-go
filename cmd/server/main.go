package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"videoflow-go/internal/agents"
	"videoflow-go/internal/api"
	"videoflow-go/internal/config"
	"videoflow-go/internal/db"
	"videoflow-go/internal/events"
	"videoflow-go/internal/jobs"
	"videoflow-go/internal/media"
	"videoflow-go/internal/providers"
)

func main() {
	log.Println("Starting VideoFlow Go Backend Engine...")

	cfg := config.Load()
	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}
	defer database.Close()
	log.Printf("SQLite initialized at %s with WAL mode", cfg.DatabasePath)

	broker := events.NewBroker()
	mediaEngine := media.NewMediaEngine()
	if mediaEngine.Available() {
		log.Println("FFmpeg detected and available for media processing")
	} else {
		log.Println("Warning: FFmpeg not detected in PATH; thumbnails and clipping disabled")
	}

	openrouter := providers.NewOpenRouterClient(cfg.OpenRouterAPIKey)
	atlascloud := providers.NewAtlasCloudClient(cfg.AtlasCloudAPIKey)
	agentsEngine := agents.NewAgentEngine(openrouter, "openai/gpt-4o-mini")

	workers := jobs.NewWorkerPool(cfg, database, broker, mediaEngine)
	workers.Start()
	workers.ReconcilePending()
	defer workers.Stop()

	server := api.NewServer(cfg, database, broker, workers, agentsEngine, openrouter, atlascloud, mediaEngine)
	addr := fmt.Sprintf(":%d", cfg.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      server.Router(),
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("VideoFlow Go server listening on http://127.0.0.1%s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("VideoFlow Go backend stopped.")
}
