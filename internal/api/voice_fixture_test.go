package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"videoflow-go/internal/models"
)

func voiceFixture(t *testing.T, handler http.HandlerFunc) *Server {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	remote := httptest.NewServer(handler)
	t.Cleanup(remote.Close)
	s.cfg.FalAPIKey = "test"
	s.cfg.FalBaseURL = remote.URL
	if err := s.database.EnsureVoiceWorkflow(); err != nil {
		t.Fatal(err)
	}
	return s
}
func voiceCharacter(t *testing.T, s *Server) string {
	project, _ := s.database.GetActiveProjectID()
	c := &models.Character{ProjectID: &project, Name: "Speaker"}
	if err := s.database.CreateCharacter(c); err != nil {
		t.Fatal(err)
	}
	return c.ID
}
