package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

// Separate counting servers prove routing rather than accepting any protocol at one endpoint.
func TestGuidedFalImagesWithSelectableFalOrBytePlusVideo(t *testing.T) {
	binary, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg required")
	}
	clip := filepath.Join(t.TempDir(), "clip.mp4")
	if b, err := exec.Command(binary, "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=320x240:d=1", "-c:v", "libx264", "-pix_fmt", "yuv420p", clip).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	video, _ := os.ReadFile(clip)
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aO1cAAAAASUVORK5CYII=")
	for _, videoProvider := range []string{"fal", "byteplus"} {
		t.Run(videoProvider, func(t *testing.T) {
			var images, falVideos, arkVideos atomic.Int32
			var falBase, arkBase string
			fal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/output.png" || r.URL.Path == "/output.mp4" {
					if r.Header.Get("Authorization") != "" {
						t.Error("key leaked to output")
					}
					if r.URL.Path == "/output.png" {
						w.Write(png)
					} else {
						w.Write(video)
					}
					return
				}
				if r.Header.Get("Authorization") != "Key fal-key" {
					t.Error("fal credentials not used")
				}
				switch {
				case r.Method == "POST" && r.URL.Path == "/fal-ai/nano-banana-2/edit":
					images.Add(1)
					var p map[string]any
					json.NewDecoder(r.Body).Decode(&p)
					if p["image_urls"] == nil || !strings.Contains(stringValueAPI(p["prompt"]), "Mimi") {
						t.Error("character reference lost", p)
					}
					w.Write([]byte(`{"request_id":"image-job"}`))
				case r.Method == "POST" && r.URL.Path == "/"+providers.FalVideoModel:
					falVideos.Add(1)
					var p map[string]any
					json.NewDecoder(r.Body).Decode(&p)
					if p["image_url"] == nil || p["duration"] != "5" || !strings.Contains(stringValueAPI(p["prompt"]), "Chinese") {
						t.Error("fal first frame/brief lost", p)
					}
					w.Write([]byte(`{"request_id":"video-job"}`))
				case strings.HasSuffix(r.URL.Path, "/status"):
					w.Write([]byte(`{"status":"COMPLETED"}`))
				case strings.HasSuffix(r.URL.Path, "/requests/image-job"):
					json.NewEncoder(w).Encode(map[string]any{"images": []any{map[string]string{"url": falBase + "/output.png"}}})
				case strings.HasSuffix(r.URL.Path, "/requests/video-job"):
					json.NewEncoder(w).Encode(map[string]any{"video": map[string]string{"url": falBase + "/output.mp4"}})
				default:
					t.Error("unexpected fal path", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer fal.Close()
			falBase = fal.URL
			ark := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/output.mp4" {
					if r.Header.Get("Authorization") != "" {
						t.Error("key leaked to ark output")
					}
					w.Write(video)
					return
				}
				if r.Header.Get("Authorization") != "Bearer ark-key" {
					t.Error("BytePlus credentials not used")
				}
				switch r.Method + " " + r.URL.Path {
				case "POST /contents/generations/tasks":
					arkVideos.Add(1)
					var p map[string]any
					json.NewDecoder(r.Body).Decode(&p)
					content, _ := p["content"].([]any)
					if len(content) != 2 || p["model"] != providers.BytePlusVideoModel {
						t.Error("direct Seedance conditioning lost", p)
					}
					w.Write([]byte(`{"id":"ark-job"}`))
				case "GET /contents/generations/tasks/ark-job":
					json.NewEncoder(w).Encode(map[string]any{"status": "succeeded", "content": map[string]string{"video_url": arkBase + "/output.mp4"}})
				default:
					t.Error("unexpected BytePlus path", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer ark.Close()
			arkBase = ark.URL
			var s *Server
			var changedDuringPreflight atomic.Bool
			s = routerServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/models":
					if changedDuringPreflight.CompareAndSwap(false, true) {
						s.database.SetAppSettings(map[string]any{"default_image_provider": "openrouter", "default_video_provider": "openrouter", "image_model": "changed-during-preflight", "video_model": "changed-during-preflight"})
					}
					w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
				case "/chat/completions":
					var p struct{ Messages []struct{ Content string } }
					json.NewDecoder(r.Body).Decode(&p)
					content := `{"prompt":"Mimi white cat in cinematic light"}`
					if strings.Contains(p.Messages[0].Content, "scriptwriter") {
						content = `{"title":"Mimi","summary":"Cat story","scenes":[{"title":"One","summary":"Mimi plays","duration":5,"aspect_ratio":"16:9"}]}`
					} else if strings.Contains(p.Messages[0].Content, "cinematographer") {
						content = `{"shots":[{"shot_order":1,"duration":5,"prompt":"Mimi plays","camera":"wide","movement":"still"}]}`
					}
					json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
				default:
					t.Error("media leaked to OpenRouter", r.URL.Path)
					http.NotFound(w, r)
				}
			})
			s.cfg.FalAPIKey = "fal-key"
			s.cfg.FalBaseURL = falBase
			s.cfg.BytePlusAPIKey = "ark-key"
			s.cfg.BytePlusBaseURL = arkBase
			model := providers.FalVideoModel
			endpoint := falBase
			if videoProvider == "byteplus" {
				model = providers.BytePlusVideoModel
				endpoint = arkBase
			}
			s.database.SetAppSettings(map[string]any{"default_image_provider": "fal", "default_video_provider": videoProvider, "image_model": providers.FalImageModel, "ref_image_model": providers.FalImageModel, "video_model": model})
			project, _ := s.database.GetActiveProjectID()
			c := &models.Character{ProjectID: &project, Name: "Mimi", Appearance: "white cat"}
			s.database.CreateCharacter(c)
			photo := filepath.Join(s.cfg.StorageRoot, "photo.png")
			os.WriteFile(photo, png, 0600)
			a := &models.Asset{ProjectID: &project, CharacterID: &c.ID, Type: "character_reference", FilePath: photo}
			if err = s.database.CreateUploadedAsset(a); err != nil {
				t.Fatal(err)
			}
			s.workers.Start()
			defer s.workers.Stop()
			r := call(t, s, "POST", "/videos/generate", fmt.Sprintf(`{"idea":"Mimi plays","scene_count":1,"target_duration":5,"aspect_ratio":"16:9","language":"Chinese","character_ids":[%q]}`, c.ID))
			if r.Code != 202 {
				t.Fatal(r.Code, r.Body.String())
			}
			// The pipeline must keep its captured route despite subsequent default edits.
			s.database.SetAppSettings(map[string]any{"default_image_provider": "openrouter", "default_video_provider": "openrouter", "image_model": "changed-image", "video_model": "changed-video"})
			deadline := time.Now().Add(10 * time.Second)
			var finished *models.RenderJob
			for time.Now().Before(deadline) {
				jobs, _ := s.database.ListRenderJobs(project)
				if len(jobs) == 1 {
					job := jobs[0]
					if job.Status == "failed" || job.Status == "reconciliation_required" {
						t.Fatal(job.Status, job.Error)
					}
					if job.Status == "succeeded" {
						finished = &job
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
			}
			if finished == nil {
				ops, _ := s.database.ListOps(project, 5)
				t.Fatalf("workflow did not finish: %+v", ops)
			}
			if finished.Provider != videoProvider || finished.ProviderEndpoint != endpoint || finished.Model != model || images.Load() != 1 {
				t.Fatalf("wrong route: %+v images %d", finished, images.Load())
			}
			if videoProvider == "fal" && (falVideos.Load() != 1 || arkVideos.Load() != 0) {
				t.Fatal("wrong video provider count")
			}
			if videoProvider == "byteplus" && (arkVideos.Load() != 1 || falVideos.Load() != 0) {
				t.Fatal("wrong direct provider count")
			}
			outputs, _ := s.database.ListOutputsForJob(finished.ID)
			if len(outputs) != 1 {
				t.Fatal("missing downloadable output")
			}
			download := call(t, s, "GET", "/outputs/"+outputs[0].ID+"/download", "")
			if download.Code != 200 || download.Body.Len() == 0 {
				t.Fatal("download failed", download.Code)
			}
		})
	}
}
func stringValueAPI(v any) string { s, _ := v.(string); return s }
