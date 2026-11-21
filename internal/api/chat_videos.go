package api

import (
	"strings"

	"videoflow-go/internal/agents"
	"videoflow-go/internal/models"
)

// The chat actions that make or rescue scene videos: render_scenes makes several at once,
// check_video and stop_waiting settle a request the video service has not answered.

// namesScene reports whether the planner's reference (an id or a title) is this scene.
func namesScene(ref string, scene models.Scene) bool {
	ref = strings.TrimSpace(ref)
	return ref != "" && (ref == scene.ID || strings.EqualFold(ref, strings.TrimSpace(scene.Title)))
}

// chatEpisodeScript is the story a whole-episode action is about: the numbered episode,
// else the episode of the named scene, else the newest one.
func chatEpisodeScript(scenes []models.Scene, episodes []seriesEpisode, res *agents.ChatIntentResponse) string {
	if res.EpisodeNumber >= 1 && res.EpisodeNumber <= len(episodes) {
		return episodes[res.EpisodeNumber-1].ScriptID
	}
	for _, scene := range scenes {
		if scene.ScriptID != nil && namesScene(res.SceneID, scene) {
			return *scene.ScriptID
		}
	}
	if len(episodes) > 0 {
		return episodes[len(episodes)-1].ScriptID
	}
	return ""
}

// chatSceneBatch is what render_scenes makes: the scenes the planner named, or else every
// scene of one episode that has no video yet (or whose last try failed). Scenes being
// made or waiting on an answer from the service are left to their own actions.
func chatSceneBatch(scenes []models.Scene, episodes []seriesEpisode, progress projectProgress, res *agents.ChatIntentResponse) []map[string]any {
	video := map[string]string{}
	for _, p := range progress.Scenes {
		video[p.SceneID] = p.Video.State
	}
	batch := []map[string]any{}
	add := func(scene models.Scene) {
		batch = append(batch, map[string]any{"id": scene.ID, "title": scene.Title, "duration": scene.Duration})
	}
	for _, scene := range scenes {
		for _, ref := range res.SceneIDs {
			if namesScene(ref, scene) {
				add(scene)
				break
			}
		}
	}
	if len(batch) > 0 || len(res.SceneIDs) > 0 {
		return batch
	}
	script := chatEpisodeScript(scenes, episodes, res)
	for _, scene := range scenes {
		if scene.ScriptID != nil && *scene.ScriptID == script && (video[scene.ID] == "none" || video[scene.ID] == "failed") {
			add(scene)
		}
	}
	return batch
}

// chatWaitingScene is the scene check_video or stop_waiting is about: the named request
// or scene, else the selected scene, else (when nothing is named) the only scene
// waiting on an answer.
func chatWaitingScene(progress projectProgress, scenes []models.Scene, res *agents.ChatIntentResponse, selected *chatSelection) *sceneProgress {
	waiting := []*sceneProgress{}
	for i := range progress.Scenes {
		if p := &progress.Scenes[i]; p.Video.State == "waiting_on_service" {
			waiting = append(waiting, p)
		}
	}
	for _, p := range waiting {
		if res.JobID != "" && res.JobID == p.Video.JobID {
			return p
		}
	}
	for _, p := range waiting {
		for _, scene := range scenes {
			if scene.ID == p.SceneID && (namesScene(res.SceneID, scene) || selected != nil && (selected.Kind == "scene" && selected.ID == scene.ID || selected.Kind == "render_job" && selected.ID == p.Video.JobID)) {
				return p
			}
		}
	}
	if len(waiting) == 1 && res.JobID == "" && res.SceneID == "" {
		return waiting[0]
	}
	return nil
}
