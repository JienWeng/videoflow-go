package api

import (
	"encoding/json"
	"fmt"
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
	case "output", "render_job":
		jobs, e := s.database.ListRenderJobs(project)
		if e == nil {
			for _, j := range jobs {
				if selected.Kind == "render_job" && j.ID == selected.ID {
					record = j
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
		return nil, fmt.Errorf("selected item is unavailable in this project; refresh the canvas")
	}
	return json.Marshal(map[string]any{"kind": selected.Kind, "id": selected.ID, "record": record, "instruction": "The user selected this item. Target this item for compatible actions. Do not silently target a different item. Explain unsupported edits. Selection alone is not approval for generation."})
}

func selectedChatActionAllowed(kind, action string) bool {
	if action == "unknown" {
		return true
	}
	switch kind {
	case "scene":
		return action == "refine_scene" || action == "render_scene" || action == "storyboard" || action == "generate_shots" || action == "delete_scene"
	case "shot":
		return action == "refine_shot" || action == "render_shot"
	case "output":
		return action == "caption" || action == "retry_render"
	case "character":
		return action == "generate_script"
	}
	return false
}
