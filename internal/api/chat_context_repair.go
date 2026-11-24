package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"net/http"
	"videoflow-go/internal/models"
)

// chatContextInvalid is the code of the failure the chat panel offers to repair.
const (
	chatContextInvalid        = "chat_context_invalid"
	chatContextInvalidMessage = "Saved chat context is out of date. Repair it to remove unavailable characters or assets, then retry your message."
)

func chatContextError(w http.ResponseWriter) {
	writeJSON(w, 409, map[string]string{"code": chatContextInvalid, "error": chatContextInvalidMessage})
}

// Explicitly removes unavailable saved context only. No turns, approval or native history are changed.
func (s *Server) handleRepairChatContext(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	id := chi.URLParam(r, "id")
	session, err := s.database.GetDirectorSession(id, project)
	if err != nil {
		http.Error(w, "session not found", 404)
		return
	}
	if session.Status == models.StatusRunning {
		http.Error(w, "wait for the current request to finish before repairing context", 409)
		return
	}
	raw, err := s.database.GetDirectorSelection(id)
	var saved directorSelection
	if err != nil || json.Unmarshal(raw, &saved) != nil {
		http.Error(w, "saved context could not be read; review it in AI session settings", 409)
		return
	}
	next := directorSelection{CharacterIDs: []string{}, AssetIDs: []string{}}
	removed := 0
	for _, cid := range saved.CharacterIDs {
		c, e := s.database.GetCharacter(cid)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			http.Error(w, "could not check saved characters; try again", 500)
			return
		}
		if e == nil && c.ProjectID != nil && *c.ProjectID == project {
			next.CharacterIDs = append(next.CharacterIDs, cid)
		} else {
			removed++
		}
	}
	for _, aid := range saved.AssetIDs {
		a, e := s.database.GetAsset(aid)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			http.Error(w, "could not check saved assets; try again", 500)
			return
		}
		if e == nil && a.ProjectID != nil && *a.ProjectID == project {
			next.AssetIDs = append(next.AssetIDs, aid)
		} else {
			removed++
		}
	}
	if _, err = s.buildDirectorContext(project, next); err != nil {
		http.Error(w, "Remaining character references need attention in Library: "+err.Error(), 409)
		return
	}
	encoded, _ := json.Marshal(next)
	ok, err := s.database.CompareDirectorSelection(id, project, raw, encoded)
	if err != nil {
		http.Error(w, "could not save repaired context", 500)
		return
	}
	if !ok {
		http.Error(w, "session context changed; reload before repairing", 409)
		return
	}
	writeJSON(w, 200, map[string]any{"removed": removed, "selection": next})
}
