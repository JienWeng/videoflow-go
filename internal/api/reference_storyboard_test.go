package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"videoflow-go/internal/models"
)

// A scene whose clips are sent its connected pictures as references never uses a
// storyboard picture, so none is made or paid for: not while the story is created, and
// not when the scene is rendered. Switching the scene to first frame makes the render
// quote one storyboard and make it then.
func TestReferenceModeStoryMakesNoStoryboardUntilFirstFrameIsChosen(t *testing.T) {
	var images atomic.Int32
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/images/models":
			w.Write([]byte(`{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"input_references":{"type":"array"},"aspect_ratio":{"type":"enum","values":["16:9","1:1"]}}}]}`))
		case "/images":
			n := images.Add(1)
			data := []byte{137, 80, 78, 71, 13, 10, 26, 10, byte(n)}
			json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(data), "media_type": "image/png"}}})
		default:
			t.Errorf("unexpected provider path %s", r.URL.Path)
			w.WriteHeader(500)
		}
	})
	// A video service that takes reference pictures; no clip is ever sent to it here.
	argo := "https://argolink.io/v1"
	if e := s.database.SetProviderSecret("argolink", "test-key", &argo); e != nil {
		t.Fatal(e)
	}
	s.database.SetAppSetting("default_video_provider", `"argolink"`)
	s.database.SetAppSetting("video_model", `"seedance-2.0"`)
	s.database.SetAppSetting("video_prices_checked", time.Now().UTC().Format(time.RFC3339))
	project, _ := s.database.GetActiveProjectID()
	s.database.CreateDirectorSession(project, "codex", "Dog story")
	fake := filepath.Join(t.TempDir(), "codex")
	os.WriteFile(fake, []byte(`#!/usr/bin/env python3
import sys,json
p=sys.stdin.read()
result={'prompt':'Pip, a small golden puppy, meets his cream-colored mother'}
if 'scriptwriter' in p: result=json.loads(`+fmt.Sprintf("%q", strings.ReplaceAll(dogStoryDraft, `"duration":1`, `"duration":4`))+`)
elif 'Plan only essential visible props' in p: result={'assets':[]}
elif 'cinematographer' in p: result={'shots':[{'shot_order':1,'duration':4,'prompt':'Pip and Mother dog meet','camera':'wide','movement':'still'}]}
print(json.dumps({'type':'thread.started','thread_id':'dog-native'}))
print(json.dumps({'type':'item.completed','item':{'type':'agent_message','text':json.dumps(result)}}))
print(json.dumps({'type':'turn.completed'}))
`), 0700)
	s.directorRunner.Codex = fake
	finished := func(id string) *models.Op {
		t.Helper()
		for until := time.Now().Add(15 * time.Second); time.Now().Before(until); {
			op, _ := s.database.GetOp(id)
			if op.Status == "failed" {
				t.Fatal("operation failed:", *op.Error)
			}
			if op.Status == "succeeded" {
				return op
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("operation did not finish")
		return nil
	}
	storyboards := func() int {
		t.Helper()
		assets, _ := s.database.ListAssets(project)
		n := 0
		for _, a := range assets {
			if a.Type == "storyboard" {
				n++
			}
		}
		return n
	}

	// (a) The story is created with its two character sheets and no storyboard.
	w := call(t, s, "POST", "/videos/generate", `{"idea":"A puppy finds his mother","scene_count":1,"target_duration":4,"aspect_ratio":"16:9","create_characters":true,"character_ids":[]}`)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var started struct {
		ID string `json:"op_id"`
	}
	json.Unmarshal(w.Body.Bytes(), &started)
	var story struct {
		Stages      []string `json:"stages"`
		Status      string   `json:"status"`
		Scenes      []string `json:"scene_ids"`
		Storyboards *int     `json:"storyboards_made"`
	}
	if op := finished(started.ID); json.Unmarshal(op.ResultJSON, &story) != nil || story.Status != "ready_to_render" || len(story.Scenes) != 1 || story.Storyboards == nil || *story.Storyboards != 0 || story.Stages[len(story.Stages)-1] != "visuals" {
		t.Fatalf("story result: %s", op.ResultJSON)
	}
	if images.Load() != 2 || storyboards() != 0 {
		t.Fatalf("reference-mode story paid for %d pictures and has %d storyboards; want the 2 character sheets only", images.Load(), storyboards())
	}
	sceneID := story.Scenes[0]
	scene, _ := s.database.GetScene(sceneID)
	if len(sceneBrief(scene).ReferenceAssetIDs) != 2 {
		t.Fatalf("the scene's brief was not saved with its cast's pictures: %+v", sceneBrief(scene))
	}

	// (b) Nothing waits on a storyboard: the scene is ready, quotes none, and renders without one.
	var progress projectProgress
	if w = call(t, s, "GET", "/production/progress", ""); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &progress) != nil || len(progress.Scenes) != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	if p := progress.Scenes[0]; p.Stage != "ready_to_render" || !p.ReferenceMode || p.Next.Action != "render_scene" || strings.Contains(strings.ToLower(p.Detail), "storyboard all match") {
		t.Fatalf("scene progress: %+v", p)
	}
	plan := func() productionPlan {
		t.Helper()
		var p productionPlan
		if w := call(t, s, "GET", "/scenes/"+sceneID+"/production", ""); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &p) != nil {
			t.Fatal(w.Code, w.Body.String())
		}
		return p
	}
	render := func(p productionPlan, storyboardCalls int) *models.Op {
		t.Helper()
		spend := productionSpend(p, false)
		w := call(t, s, "POST", "/scenes/"+sceneID+"/render?background=true", fmt.Sprintf(`{"prepare_storyboard":true,"confirmed_plan_id":%q,"confirmed_calls":%d,"confirmed_seconds":%d,"confirmed_storyboard_calls":%d}`, p.ID, spend.Calls, spend.Seconds, storyboardCalls))
		if w.Code != 202 {
			t.Fatal(w.Code, w.Body.String())
		}
		var a struct {
			ID string `json:"op_id"`
		}
		json.Unmarshal(w.Body.Bytes(), &a)
		return finished(a.ID)
	}
	references := plan()
	if spend := productionSpend(references, false); references.VideoInput != inputReferences || len(references.Pictures) != 2 || references.StoryboardReady || spend.Calls != 1 || spend.Storyboards != 0 {
		t.Fatalf("reference plan: %s pictures=%d spend=%+v", references.VideoInput, len(references.Pictures), spend)
	}
	render(references, 0)
	jobs, _ := s.database.ListRenderJobs(project)
	var request struct {
		Input  string   `json:"video_input"`
		Images []string `json:"image_urls"`
		Frames any      `json:"frame_images"`
	}
	if len(jobs) != 1 || json.Unmarshal(jobs[0].RequestJSON, &request) != nil || request.Input != inputReferences || len(request.Images) != 2 || request.Frames != nil {
		t.Fatalf("reference clip request: %d jobs %+v", len(jobs), request)
	}
	if images.Load() != 2 || storyboards() != 0 {
		t.Fatalf("rendering a reference-mode scene made a storyboard: %d pictures, %d storyboards", images.Load(), storyboards())
	}

	// (c) The owner switches the scene to first frame: one storyboard is quoted, must be
	// confirmed, and is made by the render.
	jobs[0].Status = "succeeded"
	if e := s.database.UpdateRenderJob(&jobs[0]); e != nil {
		t.Fatal(e)
	}
	settings := references.Settings
	settings.VideoInput = inputFirstFrame
	scene, _ = s.database.GetScene(sceneID)
	body, _ := json.Marshal(map[string]any{"updated_at": scene.UpdatedAt, "settings": settings})
	var frames productionPlan
	if w = call(t, s, "PUT", "/scenes/"+sceneID+"/production", string(body)); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &frames) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	spend := productionSpend(frames, false)
	if frames.VideoInput != inputFirstFrame || frames.StoryboardReady || spend.Calls != 1 || spend.Storyboards != 1 {
		t.Fatalf("first-frame plan: %s spend=%+v", frames.VideoInput, spend)
	}
	progress = projectProgress{}
	if w = call(t, s, "GET", "/production/progress", ""); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &progress) != nil || progress.Scenes[0].ReferenceMode {
		t.Fatalf("first-frame scene still shown as reference mode: %s", w.Body.String())
	}
	unconfirmed := call(t, s, "POST", "/scenes/"+sceneID+"/render?background=true", fmt.Sprintf(`{"confirmed_plan_id":%q,"confirmed_calls":1,"confirmed_seconds":%d,"confirmed_storyboard_calls":0}`, frames.ID, spend.Seconds))
	if unconfirmed.Code != 409 || images.Load() != 2 {
		t.Fatal("a storyboard that was not quoted was accepted", unconfirmed.Code, unconfirmed.Body.String())
	}
	op := render(frames, 1)
	if images.Load() != 3 || storyboards() != 1 || !strings.Contains(string(op.ResultJSON), `"storyboard_updated":true`) {
		t.Fatalf("first-frame render: %d pictures, %d storyboards, %s", images.Load(), storyboards(), op.ResultJSON)
	}
	if jobs, _ = s.database.ListRenderJobs(project); len(jobs) != 2 {
		t.Fatalf("jobs after first-frame render: %d", len(jobs))
	}
	if ready := plan(); !ready.StoryboardReady || ready.VideoInput != inputFirstFrame {
		t.Fatalf("storyboard made by the render is not current: %+v", ready)
	}
}
