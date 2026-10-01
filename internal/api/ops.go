package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleGetOp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	op, err := s.database.GetOp(id)
	if err != nil {
		http.Error(w, "op not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, op)
}

func (s *Server) handleListOps(w http.ResponseWriter, r *http.Request) {
	projectID, _ := s.database.GetActiveProjectID()
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil {
			limit = val
		}
	}

	ops, err := s.database.ListOps(projectID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, ops)
}
