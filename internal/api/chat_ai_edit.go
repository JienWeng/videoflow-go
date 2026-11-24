package api

import (
	"strings"

	"videoflow-go/internal/agents"
)

// aiEditAction asks the AI editor (ai_edit.go) to edit the finished film.
const aiEditAction = "ai_edit"

// resolveChatAIEdit points ai_edit at the episode the user means. It needs every scene
// of that episode to have its video; otherwise the action becomes unknown with a reply
// saying which scenes still need one.
func (s *Server) resolveChatAIEdit(turn *chatTurn, res *agents.ChatIntentResponse, action string, intent map[string]any) string {
	if action != aiEditAction {
		return action
	}
	script := chatEpisodeScript(turn.scenes, s.seriesEpisodes(turn.projectID), res)
	if script == "" {
		intent["action"], intent["reply"] = "unknown", "There is no finished film to edit yet. Tell me your idea and I'll plan one."
		return "unknown"
	}
	done := completedSceneRenders(turn.jobs)
	missing := []string{}
	for _, scene := range turn.scenes {
		if scene.ScriptID != nil && *scene.ScriptID == script && !done[scene.ID] {
			missing = append(missing, "“"+scene.Title+"”")
		}
	}
	if len(missing) > 0 {
		intent["action"], intent["reply"] = "unknown", "The AI editor works on the finished film, and "+strings.Join(missing, ", ")+" still need a video. Finish the scenes first."
		return "unknown"
	}
	intent["script_id"] = script
	return action
}
