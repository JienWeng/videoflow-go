package api

import (
	"net/http"

	"videoflow-go/internal/db"
)

// A selected conversation whose AI is signed out is a dead end: every message fails.
// Ensure keeps the selection when it works, and otherwise moves the project to a
// conversation on an AI that is connected (reusing one, or starting one).
func (s *Server) registerDirectorEnsureRoute() {
	s.router.Post("/director/sessions/ensure", func(w http.ResponseWriter, r *http.Request) {
		binding, _ := r.Context().Value(directorBindingKey{}).(directorBinding)
		if binding.ProjectID == "" {
			http.Error(w, "no active project", 400)
			return
		}
		sessions, err := s.database.ListDirectorSessions(binding.ProjectID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		// An engine counts as connected unless its CLI clearly says it is signed out.
		connected := map[string]bool{}
		preferred := ""
		for _, name := range []string{"claude", "codex"} {
			if _, e := s.directorRunner.Available(name); e == nil && !s.authManager().Status(r.Context(), name).SignedOut {
				connected[name] = true
				if preferred == "" {
					preferred = name
				}
			}
		}
		var current *db.DirectorSession
		for i := range sessions {
			if sessions[i].Selected {
				current = &sessions[i]
			}
		}
		if current != nil && (connected[current.Engine] || preferred == "") {
			writeJSON(w, 200, map[string]any{"session": current, "switched_from": ""})
			return
		}
		if preferred == "" {
			writeJSON(w, 200, map[string]any{"session": nil, "switched_from": ""})
			return
		}
		from := ""
		if current != nil {
			from = current.Engine
		}
		for i := range sessions {
			if connected[sessions[i].Engine] && sessions[i].Status == "ready" {
				if err = s.database.SelectDirectorSession(sessions[i].ID, binding.ProjectID); err != nil {
					http.Error(w, err.Error(), 409)
					return
				}
				sessions[i].Selected = true
				writeJSON(w, 200, map[string]any{"session": sessions[i], "switched_from": from})
				return
			}
		}
		title := "Story planning"
		if current != nil && current.Title != "" {
			title = current.Title
		}
		session, err := s.database.CreateDirectorSession(binding.ProjectID, preferred, title)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, 200, map[string]any{"session": session, "switched_from": from})
	})
}
