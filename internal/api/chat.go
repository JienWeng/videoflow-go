package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Message string              `json:"message"`
		History []map[string]string `json:"history"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	projectID, _ := s.activeProjectID(r.Context())
	scenes, _ := s.database.ListScenes(projectID)
	chars, _ := s.database.ListCharacters(projectID)
	jobs, _ := s.database.ListRenderJobs(projectID)

	outputsList := []map[string]interface{}{}
	for _, j := range jobs {
		outs, _ := s.database.ListOutputsForJob(j.ID)
		for _, o := range outs {
			outputsList = append(outputsList, map[string]interface{}{
				"id":    o.ID,
				"video": o.VideoPath,
			})
		}
	}

	sceneOptions := []map[string]interface{}{}
	for _, sc := range scenes {
		sceneOptions = append(sceneOptions, map[string]interface{}{
			"id":    sc.ID,
			"title": sc.Title,
		})
	}

	charOptions := []map[string]interface{}{}
	for _, c := range chars {
		charOptions = append(charOptions, map[string]interface{}{
			"id":   c.ID,
			"name": c.Name,
		})
	}

	contextJSON, _ := json.Marshal(map[string]any{"scenes": sceneOptions, "characters": charOptions, "outputs": outputsList})
	ctx := r.Context()
	var plannedSnapshot *directorContext
	if s.cfg.UseLocalText {
		binding, _ := ctx.Value(directorBindingKey{}).(directorBinding)
		var snapshotErr error
		plannedSnapshot, snapshotErr = s.buildDirectorContext(projectID, binding.Selection)
		if snapshotErr != nil {
			http.Error(w, snapshotErr.Error(), 409)
			return
		}
		ctx = context.WithValue(ctx, directorSnapshotKey{}, plannedSnapshot)
		body.History = nil
		ctx = context.WithValue(ctx, directorUserMessageKey{}, body.Message)
	}
	res, err := s.agents.Chat(ctx, body.Message, body.History, string(contextJSON))
	if err != nil {
		status := 500
		if errors.Is(err, errDirectorContextChanged) {
			status = 409
		}
		http.Error(w, err.Error(), status)
		return
	}

	action := canonicalChatAction(res.Action)

	intent := map[string]interface{}{
		"action":      action,
		"reply":       res.Reply,
		"idea":        body.Message,
		"scene_count": 1,
	}

	if res.SceneCount >= 1 && res.SceneCount <= 20 {
		intent["scene_count"] = res.SceneCount
	}
	for _, scene := range scenes {
		if scene.ID == res.SceneID || (res.SceneID == "" && len(scenes) == 1) {
			intent["scene_id"] = scene.ID
		}
	}
	for _, character := range chars {
		if character.ID == res.CharacterID || (res.CharacterID == "" && len(chars) == 1) {
			intent["character_id"] = character.ID
		}
	}
	for _, output := range outputsList {
		if output["id"] == res.OutputID || (res.OutputID == "" && len(outputsList) == 1) {
			intent["output_id"] = output["id"]
		}
	}

	options := map[string]interface{}{
		"scenes":         sceneOptions,
		"characters":     charOptions,
		"outputs":        outputsList,
		"caption_styles": defaultCaptionStyles,
	}

	state, err := s.projectState(projectID)
	if err != nil {
		http.Error(w, "failed to load project state", 500)
		return
	}

	response := map[string]interface{}{
		"intent":   intent,
		"options":  options,
		"state":    state,
		"warnings": []string{},
	}

	if s.cfg.UseLocalText {
		binding, _ := r.Context().Value(directorBindingKey{}).(directorBinding)
		snapshot := plannedSnapshot
		currentSelectionRaw, _ := s.database.GetDirectorSelection(binding.SessionID)
		var currentSelection directorSelection
		_ = json.Unmarshal(currentSelectionRaw, &currentSelection)
		current, err := s.buildDirectorContext(projectID, currentSelection)
		if err != nil || snapshot == nil || current.Revision != snapshot.Revision {
			http.Error(w, errDirectorContextChanged.Error(), 409)
			return
		}
		selection, _ := json.Marshal(binding.Selection)
		id, err := s.database.CreateDirectorProposal(binding.SessionID, projectID, snapshot.Revision, selection)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		intent["director_proposal"] = id
		response["session_id"] = binding.SessionID
		response["context_revision"] = snapshot.Revision
		response["warnings"] = snapshot.Warnings
	}
	writeJSON(w, http.StatusOK, response)
}

// Keep model output on the action-card contract, including legacy agent replies.
func canonicalChatAction(action string) string {
	switch action {
	case "create_script", "add_scene":
		return "generate_script"
	case "generate_script", "generate_scenes", "generate_shots", "storyboard", "render_scene", "render_shot", "caption", "retry_render", "generate_assets", "refine_scene", "refine_shot", "delete_scene", "style_ingest", "plan_assets":
		return action
	default:
		return "unknown"
	}
}
