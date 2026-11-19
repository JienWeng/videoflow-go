package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"videoflow-go/internal/models"
)

// progressNext is the single recommended step for a scene. Action names match chat action cards.
type progressNext struct {
	Action string `json:"action"`
	Label  string `json:"label"`
}

type sceneProgress struct {
	SceneID string       `json:"scene_id"`
	Title   string       `json:"title"`
	Order   int          `json:"order"`
	Stage   string       `json:"stage"` // needs_shots, needs_references, needs_storyboard, storyboard_outdated, ready_to_render, rendering, needs_attention, video_ready
	Label   string       `json:"label"`
	Detail  string       `json:"detail"`
	Shots   int          `json:"shots"`
	Step    int          `json:"step"` // 0..4 completed stages: shots, references, storyboard, video
	Next    progressNext `json:"next"`
	Blocked bool         `json:"blocked"`
	Video   sceneVideo   `json:"video"`
}

// sceneVideo is where a scene's video stands, for the assistant to act on.
type sceneVideo struct {
	State string `json:"state"` // none, making, ready, failed, waiting_on_service
	// Reason says in plain words why a video failed or what the service has not answered.
	Reason string `json:"reason,omitempty"`
	// JobID is the request the scene waits on: the one check_video or stop_waiting settles.
	JobID   string `json:"job_id,omitempty"`
	Service string `json:"service,omitempty"`
}

type projectProgress struct {
	Scenes     []sceneProgress `json:"scenes"`
	Total      int             `json:"total"`
	VideoReady int             `json:"video_ready"`
	Attention  int             `json:"attention"`
	Summary    string          `json:"summary"`
	Next       *sceneProgress  `json:"next,omitempty"`
}

// completedSceneRenders mirrors frontend/src/lib/production/completion.mjs: a planned
// scene render counts only when every unit of one plan succeeded.
func completedSceneRenders(jobs []models.RenderJob) map[string]bool {
	complete := map[string]bool{}
	type group struct {
		count int
		seen  map[int]bool
	}
	plans := map[string]*group{}
	sceneOf := map[string]string{}
	for _, job := range jobs {
		if job.Status != "succeeded" || job.SceneID == nil || job.ShotID != nil {
			continue
		}
		var r struct {
			Plan string     `json:"render_plan_id"`
			Unit renderUnit `json:"render_unit"`
		}
		_ = json.Unmarshal(job.RequestJSON, &r)
		if r.Unit.Key == "" {
			complete[*job.SceneID] = true
			continue
		}
		plan := r.Plan
		if plan == "" {
			plan = strings.SplitN(r.Unit.Key, ":", 2)[0]
		}
		if plan == "" || r.Unit.Count < 1 || r.Unit.Count > 600 || r.Unit.Index < 0 || r.Unit.Index >= r.Unit.Count {
			continue
		}
		key := *job.SceneID + ":" + plan
		g := plans[key]
		if g == nil {
			g = &group{count: r.Unit.Count, seen: map[int]bool{}}
			plans[key], sceneOf[key] = g, *job.SceneID
		}
		if g.count == r.Unit.Count {
			g.seen[r.Unit.Index] = true
		}
	}
	for key, g := range plans {
		if len(g.seen) == g.count {
			complete[sceneOf[key]] = true
		}
	}
	return complete
}

// friendlyFailure turns a stored render error into plain language without exposing provider payloads.
func friendlyFailure(job models.RenderJob) string {
	service := "The video service"
	if job.Provider != "" {
		service = videoServiceLabel(job.Provider)
	}
	switch recoveryHint(job.Error) {
	case "provider submission uncertain":
		return "The video service may have received this request. Press Check again to ask it, so you are not charged twice."
	case "check provider authentication":
		return service + " did not accept the saved key, so no video was made and nothing was charged. Paste the right key in Settings, or switch to another video service, then try again."
	case "provider rate limit":
		return "The video service is busy. Wait a minute, then try again."
	case "check provider quota":
		return service + " says the account has no balance left, so no video was made. Top it up, or switch to another video service, then try again."
	case "invalid timing":
		return "A shot length is not supported by the video model. Adjust the shot timing, then try again."
	}
	return "The last video attempt did not finish. Your shots and storyboard are kept. Ask the assistant why, or try again."
}

func (s *Server) sceneProgressFor(ctx context.Context, scene *models.Scene, jobs []models.RenderJob, complete map[string]bool) sceneProgress {
	p := sceneProgress{SceneID: scene.ID, Title: scene.Title, Order: scene.SceneOrder}
	if p.Title == "" {
		p.Title = "Untitled scene"
	}
	shots, _ := s.database.ListShots(scene.ID)
	p.Shots = len(shots)

	// Newest scene-level job decides active/failed state; render jobs are listed newest first.
	var latest *models.RenderJob
	for i := range jobs {
		j := &jobs[i]
		if j.SceneID != nil && *j.SceneID == scene.ID && j.ShotID == nil {
			if latest == nil || j.CreatedAt.After(latest.CreatedAt) {
				latest = j
			}
		}
	}
	// Any request with no answer blocks the scene, even behind a newer attempt.
	var uncertainJob *models.RenderJob
	for i := range jobs {
		if j := &jobs[i]; j.SceneID != nil && *j.SceneID == scene.ID && j.ShotID == nil && j.Status == "reconciliation_required" {
			uncertainJob = j
		}
	}
	active := latest != nil && (latest.Status == "pending" || latest.Status == "running")
	uncertain := uncertainJob != nil
	failed := latest != nil && latest.Status == "failed"
	switch {
	case uncertain:
		service := videoServiceLabel(uncertainJob.Provider)
		reason := service + " has not said whether it made this video."
		if problem := s.serviceProblem(uncertainJob.Provider); problem != "" {
			reason += " " + problem
		}
		p.Video = sceneVideo{State: "waiting_on_service", Reason: reason, JobID: uncertainJob.ID, Service: service}
	case active:
		p.Video = sceneVideo{State: "making", Service: videoServiceLabel(latest.Provider)}
	case complete[scene.ID] && !failed:
		p.Video = sceneVideo{State: "ready"}
	case failed:
		p.Video = sceneVideo{State: "failed", Reason: friendlyFailure(*latest), JobID: latest.ID, Service: videoServiceLabel(latest.Provider)}
	default:
		p.Video = sceneVideo{State: "none"}
	}

	set := func(stage, label, detail, action, actionLabel string, step int) {
		p.Stage, p.Label, p.Detail, p.Step = stage, label, detail, step
		p.Next = progressNext{Action: action, Label: actionLabel}
	}
	switch {
	case uncertain:
		set("needs_attention", "Check video result", p.Video.Reason+" Press Check again to ask, or Stop waiting to make the scene again.", "open_videos", "Check in Videos", 3)
		p.Blocked = true
		return p
	case active:
		set("rendering", "Video in progress", "The video service is working on this scene. You can keep editing other scenes.", "open_videos", "Watch progress", 3)
		return p
	}
	if complete[scene.ID] && !failed {
		set("video_ready", "Video ready", "This scene has a finished video. Add music, captions or combine scenes in the editor.", "open_editor", "Open in editor", 4)
		return p
	}
	if p.Shots == 0 {
		set("needs_shots", "Needs shots", "Break this scene into camera shots before making images or video.", "generate_shots", "Plan shots", 0)
		return p
	}
	refs, err := s.sceneReferenceState(ctx, scene)
	if err != nil {
		set("needs_attention", "Couldn't check this scene", "Refresh the page. If this keeps happening, ask the assistant to diagnose the scene.", "ask_assistant", "Ask the assistant", 1)
		return p
	}
	if missing := countIDs(refs["missing_character_ids"]) + countIDs(refs["missing_asset_ids"]); missing > 0 || refs["references_ready"] != true {
		set("needs_references", "Needs reference images", "A character, prop or location in this scene is missing an approved image, or it has more than 8 references.", "generate_assets", "Create missing images", 1)
		return p
	}
	if failed {
		set("needs_attention", "Video attempt failed", friendlyFailure(*latest), "retry_render", "Try again", 2)
		return p
	}
	boards, _ := refs["storyboards"].([]models.Asset)
	switch {
	case refs["storyboard_ready"] == true:
		set("ready_to_render", "Ready for video", "Shots, references and storyboard all match. Review the cost, then make the video.", "render_scene", "Make video", 3)
	case len(boards) > 0:
		set("storyboard_outdated", "Storyboard out of date", "The scene or its references changed after the storyboard was made. Refresh it so the video matches.", "storyboard", "Refresh storyboard", 2)
	default:
		set("needs_storyboard", "Needs storyboard", "Make a storyboard image to preview the scene before paying for video.", "storyboard", "Make storyboard", 2)
	}
	return p
}

func countIDs(v any) int {
	switch ids := v.(type) {
	case []string:
		return len(ids)
	case []any:
		return len(ids)
	}
	return 0
}

func strPtr(s string) *string { return &s }

func (s *Server) projectProgress(ctx context.Context, projectID string) (projectProgress, error) {
	out := projectProgress{Scenes: []sceneProgress{}}
	scenes, err := s.database.ListScenes(projectID)
	if err != nil {
		return out, err
	}
	jobs, err := s.database.ListRenderJobs(projectID)
	if err != nil {
		return out, err
	}
	complete := completedSceneRenders(jobs)
	for i := range scenes {
		p := s.sceneProgressFor(ctx, &scenes[i], jobs, complete)
		out.Scenes = append(out.Scenes, p)
		switch p.Stage {
		case "video_ready":
			out.VideoReady++
		case "needs_attention":
			out.Attention++
		}
	}
	out.Total = len(out.Scenes)
	// Problems first, then the earliest unfinished scene in story order.
	for i := range out.Scenes {
		if out.Scenes[i].Stage == "needs_attention" {
			out.Next = &out.Scenes[i]
			break
		}
	}
	if out.Next == nil {
		for i := range out.Scenes {
			if st := out.Scenes[i].Stage; st != "video_ready" && st != "rendering" {
				out.Next = &out.Scenes[i]
				break
			}
		}
	}
	switch {
	case out.Total == 0:
		out.Summary = "No scenes yet. Describe your story to the assistant or choose Create story."
	case out.VideoReady == out.Total:
		out.Summary = "Every scene has a video. Combine them and export in the editor."
	default:
		out.Summary = pluralScenes(out.VideoReady, out.Total) + " have video."
		if out.Attention > 0 {
			out.Summary += " " + pluralNeeds(out.Attention)
		}
	}
	return out, nil
}

func pluralScenes(done, total int) string {
	return strconv.Itoa(done) + " of " + strconv.Itoa(total) + " scenes"
}

func pluralNeeds(n int) string {
	if n == 1 {
		return "1 scene needs your attention."
	}
	return strconv.Itoa(n) + " scenes need your attention."
}

func (s *Server) handleProjectProgress(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	out, err := s.projectProgress(r.Context(), projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
