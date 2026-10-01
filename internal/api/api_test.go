package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"videoflow-go/internal/agents"
	"videoflow-go/internal/api"
	"videoflow-go/internal/config"
	"videoflow-go/internal/db"
	"videoflow-go/internal/events"
	"videoflow-go/internal/jobs"
	"videoflow-go/internal/media"
	"videoflow-go/internal/providers"
)

func setupTestServer(t *testing.T) (*api.Server, func()) {
	testDBPath := "test_api.sqlite"
	cfg := config.Load()
	cfg.DatabasePath = testDBPath

	database, err := db.Open(testDBPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	broker := events.NewBroker()
	mediaEngine := media.NewMediaEngine()
	openrouter := providers.NewOpenRouterClient(cfg.OpenRouterAPIKey)
	atlascloud := providers.NewAtlasCloudClient(cfg.AtlasCloudAPIKey)
	agentsEngine := agents.NewAgentEngine(openrouter, "openai/gpt-4o-mini")

	workers := jobs.NewWorkerPool(cfg, database, broker, mediaEngine)
	workers.Start()

	server := api.NewServer(cfg, database, broker, workers, agentsEngine, openrouter, atlascloud, mediaEngine)

	cleanup := func() {
		workers.Stop()
		database.Close()
		os.Remove(testDBPath)
	}

	return server, cleanup
}

func TestHealthEndpoint(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	server.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var res map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse json: %v", err)
	}
	if res["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", res["status"])
	}
}

func TestProjectsAndScenesEndpoints(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. Create Project
	createPayload := []byte(`{"name":"New Test Studio","description":"Testing Chi API"}`)
	req := httptest.NewRequest("POST", "/projects", bytes.NewReader(createPayload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	// 2. List Projects
	req = httptest.NewRequest("GET", "/projects", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	// 3. Create Scene
	scenePayload := []byte(`{"title":"Act 1 Scene 1","summary":"Beginning of journey","duration":10,"aspect_ratio":"16:9"}`)
	req = httptest.NewRequest("POST", "/scenes", bytes.NewReader(scenePayload))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()

	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	// 4. Graph endpoint
	req = httptest.NewRequest("GET", "/graph", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
}

func TestMissingEndpointsResolution(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// 1. GET /settings/providers/openrouter
	req := httptest.NewRequest("GET", "/settings/providers/openrouter", nil)
	rr := httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /settings/providers/openrouter, got %d", rr.Code)
	}

	// 2. GET /settings/providers/atlas
	req = httptest.NewRequest("GET", "/settings/providers/atlas", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /settings/providers/atlas, got %d", rr.Code)
	}

	// 3. GET /settings/providers
	req = httptest.NewRequest("GET", "/settings/providers", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /settings/providers, got %d", rr.Code)
	}

	// 4. GET /settings/agents
	req = httptest.NewRequest("GET", "/settings/agents", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /settings/agents, got %d", rr.Code)
	}
	var agents []map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &agents); err != nil || len(agents) < 10 {
		t.Fatalf("expected full agent list, got %v", agents)
	}

	// 5. GET /assets/types
	req = httptest.NewRequest("GET", "/assets/types", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /assets/types, got %d", rr.Code)
	}
	var typesRes map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &typesRes); err != nil || typesRes["types"] == nil {
		t.Fatalf("expected types object wrapper, got %v", typesRes)
	}

	// 6. GET /caption-styles and /caption-config
	req = httptest.NewRequest("GET", "/caption-styles", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /caption-styles, got %d", rr.Code)
	}

	req = httptest.NewRequest("GET", "/caption-config", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /caption-config, got %d", rr.Code)
	}

	// 7. Create scene and test scene action endpoints
	scenePayload := []byte(`{"title":"Act 2","summary":"Middle scene","duration":5,"aspect_ratio":"16:9"}`)
	req = httptest.NewRequest("POST", "/scenes", bytes.NewReader(scenePayload))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 for scene create, got %d", rr.Code)
	}
	var sc map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &sc)
	sceneID := sc["id"].(string)

	// POST /scenes/{id}/render
	req = httptest.NewRequest("POST", "/scenes/"+sceneID+"/render", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for scene render, got %d", rr.Code)
	}

	// POST /scenes/{id}/shots/generate?background=true
	req = httptest.NewRequest("POST", "/scenes/"+sceneID+"/shots/generate?background=true", bytes.NewReader([]byte(`{"auto_assets":true}`)))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for shots generate background, got %d", rr.Code)
	}

	// POST /scenes/{id}/storyboard?background=true
	req = httptest.NewRequest("POST", "/scenes/"+sceneID+"/storyboard?background=true", nil)
	rr = httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for storyboard background, got %d", rr.Code)
	}
}
