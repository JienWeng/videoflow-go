package models

import "encoding/json"

// CompleteSceneRenders distinguishes a finished plan from one finished subclip.
// Callers requiring playable outputs filter out jobs without saved outputs first.
func CompleteSceneRenders(jobs []RenderJob) map[string]bool {
	complete := map[string]bool{}
	type group struct {
		scene string
		count int
		seen  map[int]bool
	}
	plans := map[string]*group{}
	for _, job := range jobs {
		if job.Status != StatusSucceeded || job.SceneID == nil || job.ShotID != nil {
			continue
		}
		var saved struct {
			Unit struct {
				Key   string `json:"key"`
				Index int    `json:"index"`
				Count int    `json:"count"`
			} `json:"render_unit"`
			Plan string `json:"render_plan_id"`
		}
		if len(job.RequestJSON) > 0 && json.Unmarshal(job.RequestJSON, &saved) != nil {
			continue
		}
		if saved.Unit.Key == "" {
			complete[*job.SceneID] = true
			continue
		}
		if saved.Plan == "" || saved.Unit.Count < 1 || saved.Unit.Count > 600 || saved.Unit.Index < 0 || saved.Unit.Index >= saved.Unit.Count {
			continue
		}
		key := *job.SceneID + ":" + saved.Plan
		g := plans[key]
		if g == nil {
			g = &group{*job.SceneID, saved.Unit.Count, map[int]bool{}}
			plans[key] = g
		}
		if g.count != saved.Unit.Count {
			continue
		}
		g.seen[saved.Unit.Index] = true
	}
	for _, g := range plans {
		if len(g.seen) == g.count {
			complete[g.scene] = true
		}
	}
	return complete
}
