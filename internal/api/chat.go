package api

import (
	"encoding/json"
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

	res, err := s.agents.Chat(r.Context(), body.Message, body.History)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	projectID, _ := s.database.GetActiveProjectID()
	scenes, _ := s.database.ListScenes(projectID)
	chars, _ := s.database.ListCharacters(projectID)

	response := map[string]interface{}{
		"reply":         res.Reply,
		"action":        res.Action,
		"action_params": res.ActionParams,
		"project_state": map[string]interface{}{
			"scenes_count":     len(scenes),
			"characters_count": len(chars),
		},
		"warnings":   []string{},
		"next_steps": []string{"Refine scene details", "Submit video renders"},
	}

	writeJSON(w, http.StatusOK, response)
}
