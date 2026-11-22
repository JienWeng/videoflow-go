package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"videoflow-go/internal/models"

	"videoflow-go/internal/agents"
)

// An episode is one story (script) of the project; a project with several is a series.
type seriesEpisode struct {
	Number     int      `json:"number"`
	ScriptID   string   `json:"script_id"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	SceneCount int      `json:"scene_count"`
	CastIDs    []string `json:"cast_ids"`
}

func (s *Server) seriesEpisodes(project string) []seriesEpisode {
	scripts, _ := s.database.ListScripts(project)
	sort.SliceStable(scripts, func(i, j int) bool { return scripts[i].CreatedAt.Before(scripts[j].CreatedAt) })
	scenes, _ := s.database.ListScenes(project)
	episodes := []seriesEpisode{}
	for _, script := range scripts {
		e := seriesEpisode{ScriptID: script.ID, Title: script.Title, Summary: script.Summary, CastIDs: []string{}}
		seen := map[string]bool{}
		for _, scene := range scenes {
			if scene.ScriptID == nil || *scene.ScriptID != script.ID {
				continue
			}
			e.SceneCount++
			ids := models.IDList(scene.CharacterIDsJSON)
			for _, id := range ids {
				if !seen[id] {
					seen[id] = true
					e.CastIDs = append(e.CastIDs, id)
				}
			}
		}
		if e.SceneCount == 0 {
			continue // A story whose scenes were all removed is no longer an episode.
		}
		e.Number = len(episodes) + 1
		episodes = append(episodes, e)
	}
	return episodes
}

// lastStoryBrief is the format the most recent story was made with.
func (s *Server) lastStoryBrief(project string) *agents.CreativeBrief {
	ops, _ := s.database.ListOpsFiltered(project, "video_generation", 10)
	for _, op := range ops {
		var result struct {
			Brief *agents.CreativeBrief `json:"brief"`
		}
		if op.Status == models.StatusSucceeded && json.Unmarshal(op.ResultJSON, &result) == nil && result.Brief != nil && result.Brief.AspectRatio != "" {
			return result.Brief
		}
	}
	return nil
}

// continueSeries shapes a new story brief as the next episode: same format as the last
// one, the same cast, and a recap so the writer continues instead of starting over.
func (s *Server) continueSeries(project string, brief *agents.CreativeBrief, editing bool) (cast []string, number int) {
	episodes := s.seriesEpisodes(project)
	if len(episodes) == 0 {
		return nil, 0
	}
	number = len(episodes) + 1
	if last := s.lastStoryBrief(project); last != nil && !editing {
		// The series keeps its format; the user can still change any of it on the plan.
		brief.Platform, brief.AspectRatio, brief.Resolution, brief.Language, brief.ConversationMode = last.Platform, last.AspectRatio, last.Resolution, last.Language, last.ConversationMode
		brief.TargetDuration, brief.SceneCount = last.TargetDuration, last.SceneCount
		for _, pair := range [][2]*string{{&brief.Tone, &last.Tone}, {&brief.Audience, &last.Audience}, {&brief.PresetID, &last.PresetID}, {&brief.StorytellingID, &last.StorytellingID}} {
			if *pair[0] == "" {
				*pair[0] = *pair[1]
			}
		}
	}
	seen := map[string]bool{}
	recap := []string{}
	for _, e := range episodes {
		for _, id := range e.CastIDs {
			if !seen[id] && len(cast) < 8 {
				seen[id] = true
				cast = append(cast, id)
			}
		}
		summary := e.Summary
		if len([]rune(summary)) > 500 {
			summary = string([]rune(summary)[:500]) + "…"
		}
		recap = append(recap, fmt.Sprintf("Episode %d, \"%s\": %s", e.Number, e.Title, summary))
	}
	if len(recap) > 6 {
		recap = recap[len(recap)-6:]
	}
	note := fmt.Sprintf("This is episode %d of a series. Story so far:\n%s\nContinue with the same characters, world, look and tone. Do not retell earlier episodes: open with a hook that follows from the last one, tell one complete small story, and end with a reason to watch the next episode.", number, strings.Join(recap, "\n"))
	if !strings.Contains(brief.Instruction, "of a series. Story so far") {
		brief.Instruction = strings.TrimSpace(brief.Instruction + "\n" + note)
	}
	if len(brief.Instruction) > 4800 {
		brief.Instruction = brief.Instruction[:4800]
	}
	return cast, number
}
