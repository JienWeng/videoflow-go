package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"videoflow-go/internal/agents"
	"videoflow-go/internal/config"
	"videoflow-go/internal/db"
	"videoflow-go/internal/events"
	"videoflow-go/internal/jobs"
	"videoflow-go/internal/media"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

func TestE2EBrowserFixture(t *testing.T) {
	if os.Getenv("VIDEOFLOW_E2E_SERVE") != "1" {
		t.Skip("set VIDEOFLOW_E2E_SERVE=1 for isolated browser fixture")
	}
	binary, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("real FFmpeg fixture required")
	}
	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.mp4")
	if b, err := exec.Command(binary, "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=160x120:d=1", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:a", "aac", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-shortest", clip).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, b)
	}
	clipBytes, _ := os.ReadFile(clip)
	pngPath := filepath.Join(dir, "reference.png")
	if output, err := exec.Command(binary, "-v", "error", "-i", clip, "-frames:v", "1", pngPath).CombinedOutput(); err != nil {
		t.Fatalf("reference PNG fixture: %v %s", err, output)
	}
	pngBytes, _ := os.ReadFile(pngPath)
	var submissions, referenceImages atomic.Int32
	s := e2eRouterServer(t, filepath.Join(dir, "storage"), func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}},{"id":"openai/whisper-1","architecture":{"output_modalities":["text"],"input_modalities":["audio"]}}]}`))
		case r.URL.Path == "/chat/completions":
			t.Error("text inference escaped local fake CLI")
			http.Error(w, "unexpected remote text", 500)
		case r.URL.Path == "/audio/transcriptions":
			w.Write([]byte(`{"text":"Mimi plays.","segments":[{"start":0,"end":0.8,"text":"Mimi plays."}]}`))
		case r.URL.Path == "/images/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"aspect_ratio":{"type":"enum","values":["16:9"]},"input_references":{"type":"array","min":0,"max":8}}}]}`))
		case r.URL.Path == "/images":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			referenceImages.Add(1)
			fmt.Fprintf(w, `{"data":[{"b64_json":%q,"media_type":"image/png"}]}`, base64.StdEncoding.EncodeToString(pngBytes))
		case r.URL.Path == "/videos/models":
			w.Write([]byte(`{"data":[{"id":"google/veo-3.1-lite","supported_durations":[1],"supported_aspect_ratios":["16:9"],"supported_frame_images":["first_frame"]}]}`))
		case r.URL.Path == "/videos":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			fmt.Fprintf(w, `{"id":"upstream-%d"}`, submissions.Add(1))
		case strings.HasSuffix(r.URL.Path, "/content"):
			w.Write(clipBytes)
		case strings.HasPrefix(r.URL.Path, "/videos/upstream-"):
			w.Write([]byte(`{"status":"completed"}`))
		default:
			http.NotFound(w, r)
		}
	})
	s.cfg.StorageRoot = filepath.Join(dir, "storage")
	if err = os.MkdirAll(s.cfg.StorageRoot, 0700); err != nil {
		t.Fatal(err)
	}
	project, _ := s.database.GetActiveProjectID()
	character := &models.Character{ProjectID: &project, Name: "Mimi", Description: "A cat", Appearance: "white fur"}
	s.database.CreateCharacter(character)
	imagePath := filepath.Join(s.cfg.StorageRoot, "photo.png")
	os.WriteFile(imagePath, pngBytes, 0600)
	asset := &models.Asset{ProjectID: &project, CharacterID: &character.ID, Type: "character_reference", Name: "Photo", FilePath: imagePath}
	if err = s.database.CreateUploadedAsset(asset); err != nil {
		t.Fatal(err)
	}

	audioPath := filepath.Join(dir, "speech.mp3")
	if output, err := exec.Command(binary, "-v", "error", "-i", clip, "-vn", "-c:a", "libmp3lame", audioPath).CombinedOutput(); err != nil {
		t.Fatalf("speech fixture: %v %s", err, output)
	}
	audioBytes, _ := os.ReadFile(audioPath)
	var voiceSubmissions atomic.Int32
	var failVoice atomic.Bool
	var fal *httptest.Server
	fal = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/speech.mp3" {
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Write(audioBytes)
			return
		}
		if r.Method == "POST" {
			voiceSubmissions.Add(1)
			if failVoice.Swap(false) {
				http.Error(w, "fixture provider failure after submission", 500)
				return
			}
			w.Write([]byte(`{"request_id":"speech-fixture-receipt"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/status") {
			w.Write([]byte(`{"status":"COMPLETED"}`))
			return
		}
		fmt.Fprintf(w, `{"audio":{"url":%q}}`, fal.URL+"/speech.mp3")
	}))
	defer fal.Close()
	s.cfg.FalAPIKey = "fixture-only"
	s.cfg.FalBaseURL = fal.URL
	s.router.Post("/e2e/fal-fail-next", func(w http.ResponseWriter, r *http.Request) { failVoice.Store(true); w.Write([]byte(`{}`)) })
	s.cfg.UseLocalText = true
	fake := filepath.Join(dir, "codex-fixture")
	os.WriteFile(fake, []byte(`#!/usr/bin/env python3
import sys,json
prompt=sys.stdin.read()
if 'Plan an editable VideoFlow version-1 composition manifest only' in prompt:
 result=json.loads(prompt.rsplit('Current manifest: ',1)[1].split('\nRequested edit:',1)[0])
elif 'VideoFlow Studio assistant' in prompt:
 result={'action':'generate_script','reply':'Review this two-scene story proposal, then run it.','scene_count':2}
elif 'scriptwriter' in prompt:
 result={'title':'Mimi','summary':'Cat story','scenes':[{'title':'One','summary':'Mimi arrives','duration':1,'aspect_ratio':'16:9'},{'title':'Two','summary':'Mimi plays','duration':1,'aspect_ratio':'16:9'}]}
elif 'cinematographer' in prompt:
 result={'shots':[{'shot_order':1,'duration':1,'prompt':'Mimi plays','camera':'wide','movement':'still'}]}
else:
 result={'prompt':'Mimi the cat, cinematic framing'}
print(json.dumps({'type':'thread.started','thread_id':'native-browser-fixture'}))
print(json.dumps({'type':'item.completed','item':{'type':'agent_message','text':json.dumps(result)}}))
print(json.dumps({'type':'turn.completed'}))
`), 0700)
	s.directorRunner.Codex = fake
	session, err := s.database.CreateDirectorSession(project, "codex", "Browser production")
	if err != nil {
		t.Fatal(err)
	}
	selected, _ := json.Marshal(map[string]any{"character_ids": []string{character.ID}, "asset_ids": []string{asset.ID}})
	s.database.SetDirectorSelection(session.ID, project, selected)
	if response := call(t, s, "PUT", "/voice/profiles/"+character.ID, `{"revision":0,"data":{"mode":"saved","voice_id":"fixture-saved-voice","approved":true}}`); response.Code != 200 {
		t.Fatalf("seed voice profile: %d %s", response.Code, response.Body.String())
	}
	// Isolated inspection endpoint exposes fixture counters, never production data.
	s.router.Get("/e2e/counters", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"videos": submissions.Load(), "images": referenceImages.Load(), "voice": voiceSubmissions.Load()})
	})
	s.workers.Start()
	defer s.workers.Stop()
	listener, err := net.Listen("tcp", "127.0.0.1:18091")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1:5195")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		s.Router().ServeHTTP(w, r)
	}))
	server.Listener = listener
	server.Start()
	defer server.Close()
	defer server.CloseClientConnections()
	os.WriteFile("/tmp/videoflow-native-browser-fixture-ready", []byte("ready"), 0600)
	for i := 0; i < 4800; i++ {
		if _, err := os.Stat("/tmp/videoflow-native-browser-fixture-stop"); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Set the production-style storage root before static routes capture its path.
func e2eRouterServer(t *testing.T, root string, h http.HandlerFunc) *Server {
	t.Helper()
	remote := httptest.NewServer(h)
	t.Cleanup(remote.Close)
	cfg := config.Load()
	cfg.UseLocalText = true
	cfg.StorageRoot = root
	cfg.DatabasePath = filepath.Join(t.TempDir(), "e2e.sqlite")
	cfg.OpenRouterAPIKey = ""
	cfg.OpenRouterBaseURL = remote.URL
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err = database.SetProviderSecret("openrouter", "fixture-key", nil); err != nil {
		t.Fatal(err)
	}
	database.SetAppSettings(map[string]any{"default_image_provider": "openrouter", "default_video_provider": "openrouter"})
	broker := events.NewBroker()
	engine := media.NewMediaEngine()
	client := providers.NewOpenRouterClientAt("", remote.URL)
	server := NewServer(cfg, database, broker, jobs.NewWorkerPool(cfg, database, broker, engine), agents.NewAgentEngine(client, "openai/gpt-4o-mini"), client, engine)
	t.Cleanup(server.Close)
	return server
}
