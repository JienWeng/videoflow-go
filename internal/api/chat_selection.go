package api

import (
	"encoding/json"
)

type chatSelection struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// Resolve scope from owned server records, never client-supplied prompt/identity text.
func (s *Server) chatSelectionContext(project string, selected *chatSelection) (json.RawMessage, error) {
	if selected == nil {
		return nil, nil
	}
	var record any
	owned := false
	switch selected.Kind {
	case "scene":
		v, e := s.database.GetScene(selected.ID)
		if e == nil {
			record = v
			owned = v.ProjectID != nil && *v.ProjectID == project
		}
	case "shot":
		v, e := s.database.GetShot(selected.ID)
		if e == nil {
			scene, e := s.database.GetScene(v.SceneID)
			record = v
			owned = e == nil && scene.ProjectID != nil && *scene.ProjectID == project
		}
	case "character":
		v, e := s.database.GetCharacter(selected.ID)
		if e == nil {
			record = v
			owned = v.ProjectID != nil && *v.ProjectID == project
		}
	case "asset":
		v, e := s.database.GetAsset(selected.ID)
		if e == nil {
			record = v
			owned = v.ProjectID != nil && *v.ProjectID == project
		}
	case "final_video":
		// A joined episode export. It has no actions of its own in chat; the assistant is
		// told what it is so "this video" means something.
		v, e := s.database.GetAsset(selected.ID)
		if e == nil && v.Type == "output_video" {
			record = map[string]any{"id": v.ID, "name": v.Name, "what": "the finished, joined video of an episode", "made": v.CreatedAt}
			owned = v.ProjectID != nil && *v.ProjectID == project
		}
	case "output", "render_job":
		jobs, e := s.database.ListRenderJobs(project)
		if e == nil {
			for _, j := range jobs {
				if selected.Kind == "render_job" && j.ID == selected.ID {
					record = renderJobView(j)
					owned = true
				}
				if selected.Kind == "output" {
					outputs, _ := s.database.ListOutputsForJob(j.ID)
					for _, o := range outputs {
						if o.ID == selected.ID {
							record = o
							owned = true
						}
					}
				}
			}
		}
	}
	if !owned {
		// The item was deleted, replaced or belongs elsewhere. The message still goes
		// through, just without a selection: a stale click must never block the chat.
		return nil, nil
	}
	return json.Marshal(map[string]any{"kind": selected.Kind, "id": selected.ID, "record": record, "instruction": "The user selected this item. It is the default target for compatible actions. When the user clearly names a different scene or character, or asks for a project-level step such as story settings, do that instead, set that item's id, and say in the reply which item the button works on. Selection alone is not approval for generation."})
}

func selectedChatActionAllowed(kind, action string) bool {
	switch action {
	case "unknown", "open_editor", "finish_video", "render_scenes", "generate_character_references", "check_video", "stop_waiting", "switch_video_service", "clear_failed_videos", "set_music":
		return true
	}
	switch kind {
	case "scene":
		return action == "refine_scene" || action == "render_scene" || action == "audit_storyboard" || action == "storyboard" || action == "generate_shots" || action == "delete_scene"
	case "shot":
		return action == "refine_shot" || action == "render_shot"
	case "output":
		return action == "retry_render"
	case "character":
		return action == "generate_script" || action == "generate_character_reference" || action == "update_character"
	}
	return false
}
