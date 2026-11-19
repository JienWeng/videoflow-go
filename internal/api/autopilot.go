package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"time"

	"videoflow-go/internal/models"
)

// Autopilot finishes a story without further clicks: it renders every scene, lines the
// clips up in story order, adds captions from the script and exports the final video.
// It only calls the same steps the buttons call, so every check and spend limit applies.

func (s *Server) registerAutopilotRoute() {
	s.router.Post("/autopilot/finish", s.handleAutopilotFinish)
}

// internalCall runs one of this server's own routes in-process.
func (s *Server) internalCall(ctx context.Context, method, path string, body any) (int, []byte) {
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.router.ServeHTTP(recorder, request)
	return recorder.Code, recorder.Body.Bytes()
}

func (s *Server) handleAutopilotFinish(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	var body struct {
		ScriptID string `json:"script_id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body) != nil || body.ScriptID == "" {
		http.Error(w, "choose the story to finish", 400)
		return
	}
	scenes := []models.Scene{}
	all, _ := s.database.ListScenes(project)
	for _, scene := range all {
		if scene.ScriptID != nil && *scene.ScriptID == body.ScriptID {
			scenes = append(scenes, scene)
		}
	}
	if len(scenes) == 0 {
		http.Error(w, "that story has no scenes in this project", 404)
		return
	}
	sort.SliceStable(scenes, func(i, j int) bool { return scenes[i].SceneOrder < scenes[j].SceneOrder })
	op := &models.Op{Kind: "autopilot", ProjectID: &project}
	s.operation(w, r, op, func(ctx context.Context) (any, error) {
		stage := func(text string) {
			raw, _ := json.Marshal(map[string]any{"stage": text, "script_id": body.ScriptID})
			_ = s.database.UpdateOp(op.ID, "running", raw, nil)
		}
		sameProject := func() error {
			if active, e := s.database.GetActiveProjectID(); e != nil || active != project {
				return fmt.Errorf("the open project changed while the video was being finished; reopen it and continue from Story")
			}
			return nil
		}
		// 1. Render every scene that has no finished video yet.
		waiting := map[string][]string{} // scene id → job ids
		for i := range scenes {
			scene := &scenes[i]
			if err := sameProject(); err != nil {
				return nil, err
			}
			stage(fmt.Sprintf("Making video %d of %d: %s", i+1, len(scenes), scene.Title))
			plan, e := s.sceneProductionPlan(ctx, scene)
			if e != nil {
				return nil, fmt.Errorf("%s isn't ready for video: %w", scene.Title, e)
			}
			if plan.Blocked != "" {
				return nil, fmt.Errorf("%s isn't ready for video: %s", scene.Title, plan.Blocked)
			}
			spend := productionSpend(plan, false)
			code, raw := s.internalCall(ctx, "POST", "/scenes/"+scene.ID+"/render", map[string]any{"prepare_storyboard": true, "use_scene_cast": true, "confirmed_plan_id": plan.ID, "confirmed_calls": spend.Calls, "confirmed_seconds": spend.Seconds, "confirmed_storyboard_calls": spend.Storyboards})
			if code >= 300 {
				return nil, fmt.Errorf("couldn't start the video for %s: %s", scene.Title, strings.TrimSpace(string(raw)))
			}
			var started struct {
				JobID  string   `json:"job_id"`
				JobIDs []string `json:"job_ids"`
			}
			_ = json.Unmarshal(raw, &started)
			if len(started.JobIDs) == 0 && started.JobID != "" {
				started.JobIDs = []string{started.JobID}
			}
			waiting[scene.ID] = started.JobIDs
		}
		// 2. Wait for the video service.
		for {
			pending, done := 0, 0
			for i := range scenes {
				for _, id := range waiting[scenes[i].ID] {
					job, e := s.database.GetRenderJob(id)
					if e != nil {
						return nil, e
					}
					switch job.Status {
					case "succeeded":
						done++
					case "failed", "canceled", "reconciliation_required":
						reason := "the video service did not finish it"
						if job.Error != nil && *job.Error != "" {
							reason = *job.Error
						}
						return nil, fmt.Errorf("the video for %s didn't finish: %s. Open Videos to retry it, then finish from Edit video", scenes[i].Title, reason)
					default:
						pending++
					}
				}
			}
			if pending == 0 {
				break
			}
			stage(fmt.Sprintf("Waiting for the video service: %d of %d clips done", done, done+pending))
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("the video service is taking too long; your clips keep rendering. Check Videos, then finish in Edit video")
			case <-time.After(8 * time.Second):
			}
		}
		// 3. Put the clips in story order, caption them, save and export.
		if err := sameProject(); err != nil {
			return nil, err
		}
		stage("Putting the scenes together")
		code, raw := s.internalCall(ctx, "GET", "/scripts/"+body.ScriptID+"/composition-draft", nil)
		if code != 200 {
			return nil, fmt.Errorf("couldn't line up the scenes: %s", strings.TrimSpace(string(raw)))
		}
		var manifest map[string]any
		if json.Unmarshal(raw, &manifest) != nil {
			return nil, fmt.Errorf("couldn't read the assembled video plan")
		}
		if code, raw = s.internalCall(ctx, "POST", "/compositions/captions/from-script", map[string]any{"clips": manifest["clips"], "audio": manifest["audio"]}); code == 200 {
			var captions struct {
				Captions []any `json:"captions"`
			}
			if json.Unmarshal(raw, &captions) == nil && len(captions.Captions) > 0 {
				manifest["captions"] = captions.Captions
				manifest["caption_bindings"] = []any{}
			}
		}
		code, raw = s.internalCall(ctx, "POST", "/compositions", manifest)
		var saved struct {
			Revision string `json:"revision"`
		}
		if code >= 300 || json.Unmarshal(raw, &saved) != nil || saved.Revision == "" {
			return nil, fmt.Errorf("couldn't save the edit: %s", strings.TrimSpace(string(raw)))
		}
		stage("Exporting the final video")
		code, raw = s.internalCall(ctx, "POST", "/compositions/"+saved.Revision+"/run", map[string]string{"operation": "render", "revision": saved.Revision})
		var exported struct {
			AssetURL string        `json:"asset_url"`
			Asset    *models.Asset `json:"asset"`
			Error    string        `json:"error"`
		}
		_ = json.Unmarshal(raw, &exported)
		if code >= 300 || exported.AssetURL == "" {
			detail := exported.Error
			if detail == "" {
				detail = strings.TrimSpace(string(raw))
			}
			return nil, fmt.Errorf("the scenes are ready but the export failed (%s). Open Edit video and press Export", detail)
		}
		result := map[string]any{"stage": "Done", "script_id": body.ScriptID, "revision": saved.Revision, "video_url": exported.AssetURL, "scene_count": len(scenes)}
		if exported.Asset != nil {
			result["asset_id"] = exported.Asset.ID
		}
		return result, nil
	})
}
