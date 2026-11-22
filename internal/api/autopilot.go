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
	"sync"
	"time"

	"videoflow-go/internal/models"

	"github.com/go-chi/chi/v5"
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
	// chi reuses a routing context it finds on the request, so the caller's own route
	// (POST /autopilot/finish) would decide this call's method. Start a fresh one.
	ctx = context.WithValue(ctx, chi.RouteCtxKey, nil)
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
	// JoinOnly joins and exports what is already made and never makes a video: the run
	// is refused, naming the scenes, when one has no finished video. Free.
	JoinOnly bool `json:"join_only"`
}

// autopilotRuns are the background runs in progress, by what they make.
var autopilotRuns = struct {
	sync.Mutex
	ops map[string]string
}{ops: map[string]string{}}

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
	if body.JoinOnly {
		if problem := s.autopilotJoinOnlyProblem(project, body, scenes); problem != "" {
			http.Error(w, problem, 409)
			return
		}
	}
	// The same run asked for twice (two tabs, a double click, the assistant and a button)
	// is one run: the second caller follows the first instead of colliding with it.
	background := r.URL.Query().Get("background") == "true"
	run := autopilotRunKey(project, body, scenes)
	if background {
		autopilotRuns.Lock()
		defer autopilotRuns.Unlock()
		if id := autopilotRuns.ops[run]; id != "" {
			writeJSON(w, 202, map[string]any{"op_id": id, "status": models.StatusRunning, "already_running": true})
			return
		}
	}
	op := &models.Op{Kind: "autopilot", ProjectID: &project}
	defer func() {
		if background && op.ID != "" {
			autopilotRuns.ops[run] = op.ID
		}
	}()
	s.operation(w, r, op, func(ctx context.Context) (any, error) {
		if background {
			defer func() {
				autopilotRuns.Lock()
				delete(autopilotRuns.ops, run)
				autopilotRuns.Unlock()
			}()
		}
		runner := &autopilotRunner{s: s, op: op, project: project, body: body, scenes: scenes}
		return runner.run(ctx)
	})
}

// autopilotJoinOnlyProblem says why a join-only run is refused, or "" when it may go
// ahead: every scene needs a finished video, and only a whole episode can be joined.
func (s *Server) autopilotJoinOnlyProblem(project string, body autopilotRequest, scenes []models.Scene) string {
	jobs, _ := s.database.ListRenderJobs(project)
	done, missing := completedSceneRenders(jobs), []string{}
	for _, scene := range scenes {
		if !done[scene.ID] {
			missing = append(missing, "“"+scene.Title+"”")
		}
	}
	if !body.VideosOnly && len(missing) == 0 {
		return ""
	}
	problem := "Joining only works on a whole episode."
	if len(missing) == 1 {
		problem = missing[0] + " has no finished video yet. Make its video first, then join the episode."
	} else if len(missing) > 1 {
		problem = strings.Join(missing, ", ") + " have no finished video yet. Make their videos first, then join the episode."
	}
	return problem
}

// autopilotRunKey names what a run makes, so the same run asked for twice is found.
func autopilotRunKey(project string, body autopilotRequest, scenes []models.Scene) string {
	ids := []string{}
	for _, scene := range scenes {
		ids = append(ids, scene.ID)
	}
	return fmt.Sprintf("%s|%s|%t|%s", project, body.ScriptID, body.VideosOnly, strings.Join(ids, ","))
}

// autopilotRunner carries one autopilot run through its stages.
type autopilotRunner struct {
	s       *Server
	op      *models.Op
	project string
	body    autopilotRequest
	scenes  []models.Scene
}

// stage records the step the run is on, for the progress shown to the owner.
func (a *autopilotRunner) stage(text string) {
	raw, _ := json.Marshal(map[string]any{"stage": text, "script_id": a.body.ScriptID, "videos_only": a.body.VideosOnly})
	_ = a.s.database.UpdateOp(a.op.ID, "running", raw, nil)
}

// sameProject stops the run when the owner opened another project meanwhile.
func (a *autopilotRunner) sameProject() error {
	if active, e := a.s.database.GetActiveProjectID(); e != nil || active != a.project {
		return fmt.Errorf("the open project changed while the video was being finished; reopen it and continue from Story")
	}
	return nil
}

// run goes through the stages in order and returns the operation's result.
func (a *autopilotRunner) run(ctx context.Context) (any, error) {
	adopted, stuck, err := a.askAboutEarlierRequests(ctx)
	if err != nil {
		return stuck, err
	}
	waiting, err := a.startSceneVideos(ctx, adopted)
	if err != nil {
		return nil, err
	}
	if stuck, err := a.waitForSceneVideos(ctx, waiting); err != nil {
		return stuck, err
	}
	if a.body.VideosOnly {
		return a.videosOnlyResult(waiting), nil
	}
	return a.joinAndExport(ctx)
}

// askAboutEarlierRequests is step 1. An earlier request with no answer blocks its scene.
// Ask the service about each one first (free), so the run stops before spending anything
// if one stays unanswered. It returns, by scene id, the jobs the service turned out to
// have; a scene that stays unanswered ends the run with stuckScene's result.
func (a *autopilotRunner) askAboutEarlierRequests(ctx context.Context) (map[string][]string, any, error) {
	scenes := a.scenes
	adopted := map[string][]string{} // scene id → jobs the service turned out to have
	jobs, _ := a.s.database.ListRenderJobs(a.project)
	for i := range scenes {
		for j := range jobs {
			if job := &jobs[j]; job.SceneID != nil && *job.SceneID == scenes[i].ID && job.Status == models.StatusReconciliationRequired {
				a.stage("Asking " + videoServiceLabel(job.Provider) + " about an earlier request for " + scenes[i].Title)
				switch check := a.s.checkUncertainRender(ctx, job); check.Outcome {
				case "found":
					adopted[scenes[i].ID] = append(adopted[scenes[i].ID], job.ID)
				case "unknown":
					stuck, err := stuckScene(scenes[i].Title, check)
					return nil, stuck, err
				}
			}
		}
	}
	return adopted, nil, nil
}

// startSceneVideos is step 2. Start every scene that has no finished video yet. The
// video service makes them side by side while the run moves on to the next scene. It
// returns the job ids to wait on, by scene id.
func (a *autopilotRunner) startSceneVideos(ctx context.Context, adopted map[string][]string) (map[string][]string, error) {
	scenes := a.scenes
	waiting := map[string][]string{} // scene id → job ids
	for i := range scenes {
		scene := &scenes[i]
		if err := a.sameProject(); err != nil {
			return nil, err
		}
		a.stage(fmt.Sprintf("Making video %d of %d: %s", i+1, len(scenes), scene.Title))
		plan, e := a.s.sceneProductionPlan(ctx, scene)
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
		code, raw := a.s.internalCall(ctx, "POST", "/scenes/"+scene.ID+"/render", map[string]any{"prepare_storyboard": true, "use_scene_cast": true, "confirmed_plan_id": plan.ID, "confirmed_calls": spend.Calls, "confirmed_seconds": spend.Seconds, "confirmed_storyboard_calls": spend.Storyboards})
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
	return waiting, nil
}

// waitForSceneVideos is step 3. Wait for the video service. A request that loses its
// answer is asked about again.
func (a *autopilotRunner) waitForSceneVideos(ctx context.Context, waiting map[string][]string) (any, error) {
	for {
		pending, done, stuck, err := a.countSceneVideos(ctx, waiting)
		if err != nil {
			return stuck, err
		}
		if pending == 0 {
			return nil, nil
		}
		a.stage(fmt.Sprintf("Waiting for the video service: %d of %d clips done", done, done+pending))
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("the video service is taking too long; your clips keep rendering. Ask me again in a few minutes")
		case <-time.After(autopilotPoll):
		}
	}
}

// countSceneVideos looks at every started job once and counts the clips still being
// made and those finished. A failed or unanswered clip ends the run.
func (a *autopilotRunner) countSceneVideos(ctx context.Context, waiting map[string][]string) (pending, done int, stuck any, err error) {
	scenes := a.scenes
	for i := range scenes {
		for _, id := range waiting[scenes[i].ID] {
			job, e := a.s.database.GetRenderJob(id)
			if e != nil {
				return 0, 0, nil, e
			}
			if job.Status == models.StatusReconciliationRequired {
				if check := a.s.checkUncertainRender(ctx, job); check.Outcome == "unknown" {
					stuck, err := stuckScene(scenes[i].Title, check)
					return 0, 0, stuck, err
				}
				job, _ = a.s.database.GetRenderJob(id)
			}
			switch job.Status {
			case models.StatusSucceeded:
				done++
			case models.StatusFailed, "canceled", models.StatusReconciliationRequired:
				return 0, 0, nil, fmt.Errorf("the video for %s didn't finish. %s", scenes[i].Title, friendlyFailure(*job))
			default:
				pending++
			}
		}
	}
	return pending, done, nil, nil
}

// videosOnlyResult is how a videos-only run ends once the scene videos are made.
func (a *autopilotRunner) videosOnlyResult(waiting map[string][]string) map[string]any {
	ids, titles, jobIDs := []string{}, []string{}, []string{}
	for _, scene := range a.scenes {
		ids, titles = append(ids, scene.ID), append(titles, scene.Title)
		jobIDs = append(jobIDs, waiting[scene.ID]...)
	}
	return map[string]any{"stage": "Done", "videos_only": true, "script_id": a.body.ScriptID, "scene_ids": ids, "scene_titles": titles, "job_ids": jobIDs, "scene_count": len(a.scenes)}
}

// joinAndExport is step 4. Put the clips in story order, caption them, save and export.
func (a *autopilotRunner) joinAndExport(ctx context.Context) (any, error) {
	if err := a.sameProject(); err != nil {
		return nil, err
	}
	a.stage("Putting the scenes together")
	revision, err := a.saveCaptionedComposition(ctx)
	if err != nil {
		return nil, err
	}
	a.stage("Exporting the final video")
	code, raw := a.s.internalCall(ctx, "POST", "/compositions/"+revision+"/run", map[string]string{"operation": "render", "revision": revision})
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
	result := map[string]any{"stage": "Done", "script_id": a.body.ScriptID, "revision": revision, "video_url": exported.AssetURL, "scene_count": len(a.scenes)}
	if exported.Asset != nil {
		result["asset_id"] = exported.Asset.ID
	}
	return result, nil
}

// saveCaptionedComposition lines the story's clips up, adds captions from the script
// when they can be made, and saves the edit. It returns the saved revision.
func (a *autopilotRunner) saveCaptionedComposition(ctx context.Context) (string, error) {
	code, raw := a.s.internalCall(ctx, "GET", "/scripts/"+a.body.ScriptID+"/composition-draft", nil)
	if code != 200 {
		return "", fmt.Errorf("couldn't line up the scenes: %s", strings.TrimSpace(string(raw)))
	}
	var manifest map[string]any
	if json.Unmarshal(raw, &manifest) != nil {
		return "", fmt.Errorf("couldn't read the assembled video plan")
	}
	if code, raw = a.s.internalCall(ctx, "POST", "/compositions/captions/from-script", map[string]any{"clips": manifest["clips"], "audio": manifest["audio"]}); code == 200 {
		var captions struct {
			Captions []any `json:"captions"`
		}
		if json.Unmarshal(raw, &captions) == nil && len(captions.Captions) > 0 {
			manifest["captions"] = captions.Captions
		}
	}
	code, raw = a.s.internalCall(ctx, "POST", "/compositions", manifest)
	var saved struct {
		Revision string `json:"revision"`
	}
	if code >= 300 || json.Unmarshal(raw, &saved) != nil || saved.Revision == "" {
		return "", fmt.Errorf("couldn't save the edit: %s", strings.TrimSpace(string(raw)))
	}
	return saved.Revision, nil
}
