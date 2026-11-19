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

// Autopilot makes scene videos without further clicks. Finishing a story renders every
// scene, lines the clips up in story order, adds captions from the script and exports
// the final video; a videos-only run stops after the chosen scenes are made. It only
// calls the same steps the buttons call, so every check and spend limit applies.

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

// autopilotRequest says what one run makes. A story alone (finish_video): every scene
// of it, then join, caption and export. VideosOnly (render_scenes): stop once the scene
// videos are made, for the chosen SceneIDs or, when none are named, the whole story.
type autopilotRequest struct {
	ScriptID   string   `json:"script_id"`
	SceneIDs   []string `json:"scene_ids"`
	VideosOnly bool     `json:"videos_only"`
}

// autopilotPoll is how often the run looks at the video service's progress.
var autopilotPoll = 8 * time.Second

// autopilotScenes resolves a request to the scenes it makes, in story order.
func (s *Server) autopilotScenes(project string, body autopilotRequest) ([]models.Scene, int, string) {
	if !body.VideosOnly && body.ScriptID == "" {
		return nil, 400, "choose the story to finish"
	}
	if !body.VideosOnly && len(body.SceneIDs) > 0 {
		return nil, 400, "a finished video joins the whole story, so choose the story rather than single scenes"
	}
	if body.ScriptID == "" && len(body.SceneIDs) == 0 {
		return nil, 400, "choose the scenes to make"
	}
	wanted := map[string]bool{}
	for _, id := range body.SceneIDs {
		wanted[id] = true
	}
	scenes := []models.Scene{}
	all, _ := s.database.ListScenes(project)
	for _, scene := range all {
		inStory := body.ScriptID == "" || scene.ScriptID != nil && *scene.ScriptID == body.ScriptID
		if inStory && (len(wanted) == 0 || wanted[scene.ID]) {
			scenes = append(scenes, scene)
		}
	}
	if len(scenes) == 0 {
		return nil, 404, "that story has no scenes in this project"
	}
	if len(wanted) > 0 && len(scenes) != len(wanted) {
		return nil, 404, "a chosen scene is not in this project"
	}
	sort.SliceStable(scenes, func(i, j int) bool { return scenes[i].SceneOrder < scenes[j].SceneOrder })
	return scenes, 0, ""
}

// stuckScene is how a run ends when a scene waits on a request the video service has not
// answered: the owner picks Check again or Stop waiting.
func stuckScene(title string, check renderCheck) (any, error) {
	return map[string]any{"stage": "Waiting on " + check.Service, "stuck": check}, fmt.Errorf("“%s” is waiting on an earlier request to %s that has no answer yet. %s Or press Stop waiting to make the scene again; %s might still bill the earlier request.", title, check.Service, check.Message, check.Service)
}

func (s *Server) handleAutopilotFinish(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	var body autopilotRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body) != nil {
		http.Error(w, "choose the story to finish", 400)
		return
	}
	scenes, code, problem := s.autopilotScenes(project, body)
	if code != 0 {
		http.Error(w, problem, code)
		return
	}
	op := &models.Op{Kind: "autopilot", ProjectID: &project}
	s.operation(w, r, op, func(ctx context.Context) (any, error) {
		stage := func(text string) {
			raw, _ := json.Marshal(map[string]any{"stage": text, "script_id": body.ScriptID, "videos_only": body.VideosOnly})
			_ = s.database.UpdateOp(op.ID, "running", raw, nil)
		}
		sameProject := func() error {
			if active, e := s.database.GetActiveProjectID(); e != nil || active != project {
				return fmt.Errorf("the open project changed while the video was being finished; reopen it and continue from Story")
			}
			return nil
		}
		// 1. An earlier request with no answer blocks its scene. Ask the service about each
		// one first (free), so the run stops before spending anything if one stays unanswered.
		adopted := map[string][]string{} // scene id → jobs the service turned out to have
		jobs, _ := s.database.ListRenderJobs(project)
		for i := range scenes {
			for j := range jobs {
				if job := &jobs[j]; job.SceneID != nil && *job.SceneID == scenes[i].ID && job.Status == "reconciliation_required" {
					stage("Asking " + videoServiceLabel(job.Provider) + " about an earlier request for " + scenes[i].Title)
					switch check := s.checkUncertainRender(ctx, job); check.Outcome {
					case "found":
						adopted[scenes[i].ID] = append(adopted[scenes[i].ID], job.ID)
					case "unknown":
						return stuckScene(scenes[i].Title, check)
					}
				}
			}
		}
		// 2. Start every scene that has no finished video yet. The video service makes
		// them side by side while the run moves on to the next scene.
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
			if plan.Blocked != "" && len(adopted[scene.ID]) > 0 {
				waiting[scene.ID] = adopted[scene.ID] // the service already has this scene's video
				continue
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
		// 3. Wait for the video service. A request that loses its answer is asked about again.
		for {
			pending, done := 0, 0
			for i := range scenes {
				for _, id := range waiting[scenes[i].ID] {
					job, e := s.database.GetRenderJob(id)
					if e != nil {
						return nil, e
					}
					if job.Status == "reconciliation_required" {
						if check := s.checkUncertainRender(ctx, job); check.Outcome == "unknown" {
							return stuckScene(scenes[i].Title, check)
						}
						job, _ = s.database.GetRenderJob(id)
					}
					switch job.Status {
					case "succeeded":
						done++
					case "failed", "canceled", "reconciliation_required":
						return nil, fmt.Errorf("the video for %s didn't finish. %s", scenes[i].Title, friendlyFailure(*job))
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
				return nil, fmt.Errorf("the video service is taking too long; your clips keep rendering. Ask me again in a few minutes")
			case <-time.After(autopilotPoll):
			}
		}
		if body.VideosOnly {
			ids, titles, jobIDs := []string{}, []string{}, []string{}
			for _, scene := range scenes {
				ids, titles = append(ids, scene.ID), append(titles, scene.Title)
				jobIDs = append(jobIDs, waiting[scene.ID]...)
			}
			return map[string]any{"stage": "Done", "videos_only": true, "script_id": body.ScriptID, "scene_ids": ids, "scene_titles": titles, "job_ids": jobIDs, "scene_count": len(scenes)}, nil
		}
		// 4. Put the clips in story order, caption them, save and export.
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
