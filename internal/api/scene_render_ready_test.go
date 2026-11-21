package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"videoflow-go/internal/models"
)

func TestPrepareRenderRefreshesStaleStoryboardAndReusesMatching(t *testing.T) {
	for _, testCase := range []string{"ready", "image_failure", "changed_during_image", "saved_audio"} {
		fail := testCase == "image_failure"
		t.Run(testCase, func(t *testing.T) {
			var images atomic.Int32
			var prompt string
			var during func()
			s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/videos/models":
					w.Write([]byte(`{"data":[{"id":"google/veo-3.1-lite","supported_durations":[1],"supported_aspect_ratios":["16:9"],"supported_frame_images":["first_frame"]}]}`))
				case "/images/models":
					w.Write([]byte(`{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"aspect_ratio":{"type":"enum","values":["16:9"]}}}]}`))
				case "/images":
					images.Add(1)
					if during != nil {
						during()
					}
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					prompt, _ = body["prompt"].(string)
					if fail {
						http.Error(w, "fixture image failure", 502)
						return
					}
					w.Write([]byte(`{"data":[{"b64_json":"iVBORw0KGgo=","media_type":"image/png"}]}`))
				default:
					t.Error("unexpected provider", r.URL.Path)
					w.WriteHeader(500)
				}
			})
			project, _ := s.database.GetActiveProjectID()
			s.database.CreateDirectorSession(project, "codex", "Render recovery")
			fake := filepath.Join(t.TempDir(), "codex")
			os.WriteFile(fake, []byte(`#!/usr/bin/env python3
import sys,json
p=sys.stdin.read()
print(json.dumps({'type':'thread.started','thread_id':'render-recovery'}))
print(json.dumps({'type':'item.completed','item':{'type':'agent_message','text':json.dumps({'prompt':p})}}))
print(json.dumps({'type':'turn.completed'}))
`), 0700)
			s.directorRunner.Codex = fake
			scene := &models.Scene{ProjectID: &project, Title: "Home", Summary: "Pip finds Mama", Duration: 1, AspectRatio: "16:9", CharacterIDsJSON: []byte(`[]`), SceneJSON: []byte(`{}`)}
			s.database.CreateScene(scene)
			shot := &models.Shot{SceneID: scene.ID, ShotOrder: 1, Duration: 1, Prompt: "Pip follows the fresh pawprints"}
			s.database.CreateShot(shot)
			scene, _ = s.database.GetScene(scene.ID)
			os.WriteFile(filepath.Join(s.cfg.StorageRoot, "old.png"), []byte{137, 80, 78, 71, 13, 10, 26, 10}, 0600)
			old := &models.Asset{ProjectID: &project, Type: "storyboard", Name: "Old board", FilePath: "storage/old.png", MetadataJSON: []byte(`{"scene_id":"` + scene.ID + `","scene_input_revision":"old","consistency_review":"ready"}`)}
			s.database.CreateAsset(old)
			scene.AssetIDsJSON, _ = json.Marshal([]string{old.ID})
			s.database.UpdateScene(scene)
			run := func() *models.Op {
				t.Helper()
				r := call(t, s, "POST", "/scenes/"+scene.ID+"/render?background=true", `{"prepare_storyboard":true}`)
				if r.Code != 202 {
					t.Fatal(r.Code, r.Body.String())
				}
				var a struct {
					ID string `json:"op_id"`
				}
				json.Unmarshal(r.Body.Bytes(), &a)
				for until := time.Now().Add(5 * time.Second); time.Now().Before(until); {
					op, _ := s.database.GetOp(a.ID)
					if op.Status != "running" {
						return op
					}
					time.Sleep(10 * time.Millisecond)
				}
				t.Fatal("operation timeout")
				return nil
			}
			if testCase == "changed_during_image" {
				during = func() {
					current, _ := s.database.GetScene(scene.ID)
					current.Summary = "Edited while image generated"
					s.database.UpdateScene(current)
				}
			}
			blocked := call(t, s, "POST", "/scenes/"+scene.ID+"/render", `{}`)
			if blocked.Code != 400 || images.Load() != 0 {
				t.Fatal("stale reference protection bypassed", blocked.Code, blocked.Body.String())
			}
			if testCase == "saved_audio" {
				// A scene saved when voices could be added afterwards: a speaker with no
				// saved voice, and the "saved" sound choice. It renders as a silent clip.
				c := &models.Character{ProjectID: &project, Name: "Narrator"}
				s.database.CreateCharacter(c)
				current, _ := s.database.GetScene(scene.ID)
				current.SceneJSON = voiceJSON(map[string]any{"production": map[string]any{"audio_mode": "saved", "strategy": "auto"}, "spoken_lines": []SpokenLine{{ID: "u", CharacterID: c.ID, Text: "Come home, Pip!", Language: "en"}}})
				s.database.UpdateScene(current)
			}
			op := run()
			jobs, _ := s.database.ListRenderJobs(project)
			if fail || testCase == "changed_during_image" {
				if op.Status != "failed" || len(jobs) != 0 {
					t.Fatal("failed image queued video", op.Status, len(jobs))
				}
				if testCase == "changed_during_image" {
					current, _ := s.database.GetScene(scene.ID)
					if current.Summary != "Edited while image generated" {
						t.Fatal("concurrent scene edit overwritten")
					}
				}
				return
			}
			if op.Status != "succeeded" || images.Load() != 1 || len(jobs) != 1 {
				t.Fatal(op.Status, op.Error, images.Load(), len(jobs))
			}
			if testCase == "saved_audio" && !strings.Contains(string(jobs[0].RequestJSON), `"audio_mode":"silent"`) {
				t.Fatal("saved sound choice was not rendered silent", string(jobs[0].RequestJSON))
			}
			if !strings.Contains(prompt, shot.Prompt) {
				t.Fatal("new keyframe ignored current shots", prompt)
			}
			saved, _ := s.database.GetScene(scene.ID)
			state, e := s.sceneReferenceState(context.Background(), saved)
			if e != nil || state["storyboard_ready"] != true {
				t.Fatal("generated board still stale", state, e)
			}
			if _, e = s.database.GetAsset(old.ID); e != nil {
				t.Fatal("old image deleted")
			}
			op = run()
			if op.Status != "succeeded" || images.Load() != 1 {
				t.Fatal("matching image regenerated", op.Status, op.Error, images.Load())
			}
			jobs, _ = s.database.ListRenderJobs(project)
			if len(jobs) != 1 {
				t.Fatalf("second scene render created %d paid jobs instead of reusing one", len(jobs))
			}
			var result struct {
				Reused bool `json:"reused"`
			}
			if json.Unmarshal(op.ResultJSON, &result) != nil || !result.Reused {
				t.Fatal("second action did not disclose reuse", string(op.ResultJSON))
			}

		})
	}
}
