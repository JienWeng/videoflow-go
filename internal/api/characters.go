package api

import (
	"encoding/json"
	"net/http"

	"videoflow-go/internal/models"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListCharacters(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	chars, err := s.database.ListCharacters(projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if chars == nil {
		chars = []models.Character{}
	}
	writeJSON(w, http.StatusOK, chars)
}

func (s *Server) handleGetCharacter(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, err := s.database.GetCharacter(id)
	if err != nil {
		http.Error(w, "character not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleCreateCharacter(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.database.GetActiveProjectID()
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}
	var c models.Character
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	c.ProjectID = &projectID
	if err := s.database.CreateCharacter(&c); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleUpdateCharacter(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, err := s.database.GetCharacter(id)
	if err != nil {
		http.Error(w, "character not found", http.StatusNotFound)
		return
	}
	if err := json.NewDecoder(r.Body).Decode(c); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	c.ID = id
	if err := s.database.UpdateCharacter(c); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteCharacter(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.database.DeleteCharacter(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleCharacterRefSheets(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, err := s.database.GetCharacter(id)
	if err != nil {
		http.Error(w, "character not found", http.StatusNotFound)
		return
	}

	// Trigger async character reference generation or return prompt
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":       "generating",
		"character_id": c.ID,
		"message":      "Generating character reference sheet",
	})
}
