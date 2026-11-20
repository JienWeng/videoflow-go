package api

import (
	"net/http"
)

func (s *Server) handleGetGraph(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", http.StatusInternalServerError)
		return
	}

	graphData, err := s.database.GetGraphData(projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.storyGraph(projectID, graphData)
	writeJSON(w, http.StatusOK, graphData)
}
