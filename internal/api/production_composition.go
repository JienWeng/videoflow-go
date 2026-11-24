package api

import (
	"context"
	"fmt"
	"github.com/go-chi/chi/v5"
	"math"
	"net/http"
	"sort"
	"strings"
	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
)

// Builds an unsaved local edit only; reading a draft never generates or exports media.
func (s *Server) sceneCompositionDraft(ctx context.Context, scene *models.Scene) (composition.Manifest, error) {
	m := composition.Manifest{Version: 1, Width: 720, Height: 1280, FPS: 24, Duration: float64(scene.Duration), CaptionStyle: "default", Clips: []composition.Clip{}, Audio: []composition.Clip{}, Captions: []composition.Cue{}}
	switch scene.AspectRatio {
	case "16:9":
		m.Width, m.Height = 1280, 720
	case "1:1":
		m.Width, m.Height = 720, 720
	case "4:3":
		m.Width, m.Height = 960, 720
	case "3:4":
		m.Width, m.Height = 720, 960
	case "21:9":
		m.Width, m.Height = 1680, 720
	}
	if sceneBrief(scene).Resolution == "480p" {
		m.Width = int(math.Round(float64(m.Width)*480/720/2)) * 2
		m.Height = int(math.Round(float64(m.Height)*480/720/2)) * 2
	}
	paths, _, err := s.compositionSources(*scene.ProjectID)
	if err != nil {
		return m, err
	}
	jobs, err := s.database.ListRenderJobs(*scene.ProjectID)
	if err != nil {
		return m, err
	}
	plan, e := s.sceneProductionPlan(ctx, scene)
	selected := []struct {
		job  models.RenderJob
		unit renderUnit
	}{}
	if e == nil {
		hasPlan := false
		for _, job := range jobs {
			if job.SceneID != nil && *job.SceneID == scene.ID {
				u := renderRequestUnit(job.RequestJSON)
				if u.Key != "" {
					hasPlan = true
				}
			}
		}
		for _, unit := range plan.Units {
			found := false
			for _, job := range jobs {
				if job.SceneID == nil || *job.SceneID != scene.ID || job.Status != models.StatusSucceeded {
					continue
				}
				u := renderRequestUnit(job.RequestJSON)
				if u.Key == unit.Key {
					selected = append(selected, struct {
						job  models.RenderJob
						unit renderUnit
					}{job, unit})
					found = true
					break
				}
			}
			if !found && hasPlan {
				// The scene changed after it was rendered. Edit the latest finished take
				// instead of refusing; the Story view already says a new render is suggested.
				selected = latestCompleteScenePlan(jobs, scene.ID)
				break
			}
		}
	}
	// Legacy whole-scene versions are alternatives: load only the latest, never concatenate them.
	if len(selected) == 0 {
		for _, job := range jobs {
			if job.SceneID != nil && *job.SceneID == scene.ID && job.ShotID == nil && job.Status == models.StatusSucceeded && renderRequestUnit(job.RequestJSON).Key == "" {
				selected = append(selected, struct {
					job  models.RenderJob
					unit renderUnit
				}{job, renderUnit{Length: scene.Duration}})
				break
			}
		}
	}
	if len(selected) == 0 {
		return m, fmt.Errorf("no completed matching scene videos; render the scene before assembling")
	}
	for _, item := range selected {
		outputs, err := s.database.ListOutputsForJob(item.job.ID)
		if err != nil {
			return m, err
		}
		if len(outputs) == 0 {
			return m, fmt.Errorf("saved output is missing for clip %d", item.unit.Index+1)
		}
		source := "output-" + outputs[0].ID
		path, ok := paths[source]
		if !ok {
			return m, fmt.Errorf("clip source is unavailable in this project")
		}
		duration, w, h, audio, err := composition.Probe(ctx, path)
		if err != nil {
			return m, err
		}
		if duration+0.05 < float64(item.unit.Length) {
			return m, fmt.Errorf("clip %d is shorter than its planned edit length; trim or replace it manually", item.unit.Index+1)
		}
		m.Clips = append(m.Clips, composition.Clip{ID: fmt.Sprintf("clip-%s-%d", scene.ID, item.unit.Index), AssetID: source, Start: float64(item.unit.Start), Duration: float64(item.unit.Length), OriginalDuration: duration, Speed: 1, HasAudio: audio, Width: w, Height: h})
	}
	return m, nil
}

// latestCompleteScenePlan returns the newest render plan of a scene whose clips all
// finished, in clip order. Jobs are listed newest first.
func latestCompleteScenePlan(jobs []models.RenderJob, sceneID string) []struct {
	job  models.RenderJob
	unit renderUnit
} {
	type item = struct {
		job  models.RenderJob
		unit renderUnit
	}
	plans := map[string][]item{}
	order := []string{}
	for _, job := range jobs {
		if job.SceneID == nil || *job.SceneID != sceneID || job.ShotID != nil || job.Status != models.StatusSucceeded {
			continue
		}
		r := renderRequestRef(job.RequestJSON)
		if r.Unit.Key == "" || r.Unit.Count < 1 {
			continue
		}
		plan := r.Plan
		if plan == "" {
			plan = strings.SplitN(r.Unit.Key, ":", 2)[0]
		}
		if _, seen := plans[plan]; !seen {
			order = append(order, plan)
		}
		plans[plan] = append(plans[plan], item{job, r.Unit})
	}
	for _, plan := range order {
		items, byIndex := plans[plan], map[int]item{}
		for _, it := range items {
			if _, dup := byIndex[it.unit.Index]; !dup { // newest take of each clip
				byIndex[it.unit.Index] = it
			}
		}
		count := items[0].unit.Count
		if len(byIndex) != count {
			continue
		}
		out := make([]item, 0, count)
		for i := 0; i < count; i++ {
			it, ok := byIndex[i]
			if !ok {
				break
			}
			out = append(out, it)
		}
		if len(out) == count {
			return out
		}
	}
	return nil
}

func (s *Server) handleSceneCompositionDraft(w http.ResponseWriter, r *http.Request) {
	scene, ok := s.productionScene(w, r)
	if !ok {
		return
	}
	draft, err := s.sceneCompositionDraft(r.Context(), scene)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	writeJSON(w, 200, draft)
}
func (s *Server) handleScriptCompositionDraft(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	scripts, e := s.database.ListScripts(project)
	var script *models.Script
	if e == nil {
		for i := range scripts {
			if scripts[i].ID == chi.URLParam(r, "id") {
				script = &scripts[i]
				break
			}
		}
	}
	if script == nil {
		http.Error(w, "story is not in the active project", 404)
		return
	}
	scenes, err := s.database.ListScenes(project)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	sort.SliceStable(scenes, func(i, j int) bool { return scenes[i].SceneOrder < scenes[j].SceneOrder })
	var result composition.Manifest
	for i := range scenes {
		scene := &scenes[i]
		if scene.ScriptID == nil || *scene.ScriptID != script.ID {
			continue
		}
		draft, e := s.sceneCompositionDraft(r.Context(), scene)
		if e != nil {
			http.Error(w, scene.Title+": "+e.Error(), 409)
			return
		}
		if result.Version == 0 {
			result = draft
			continue
		}
		if result.Width != draft.Width || result.Height != draft.Height {
			http.Error(w, "scene formats differ; match scene formats before assembling the story", 409)
			return
		}
		for _, clip := range draft.Clips {
			clip.Start += result.Duration
			result.Clips = append(result.Clips, clip)
		}
		result.Duration += draft.Duration
	}
	if result.Version == 0 || result.Duration > 600 {
		http.Error(w, "story has no completed scenes or exceeds the 600-second editor limit", 409)
		return
	}
	// One music bed under the whole story, lowered under speech, and fades at the cuts
	// (soundtrack.go). Reading a draft makes no music: only a track already there plays.
	result, _ = s.scoreStory(r.Context(), project, script.ID, result)
	writeJSON(w, 200, result)
}
