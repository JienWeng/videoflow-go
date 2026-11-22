package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"videoflow-go/internal/agents"
	"videoflow-go/internal/models"
)

func TestRecoverySkillInstalledInResumedWorkspaces(t *testing.T) {
	s := routerServer(t, nil)
	project, _ := s.database.GetActiveProjectID()
	for _, engine := range []string{"claude", "codex"} {
		session, e := s.database.CreateDirectorSession(project, engine, "Recovery")
		if e != nil {
			t.Fatal(e)
		}
		base := filepath.Join(filepath.Dir(s.cfg.DatabasePath), ".videoflow-agent-workspaces", project, session.ID)
		if e = os.MkdirAll(base, 0700); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(base, ".videoflow-skills-v1"), []byte("existing"), 0600); e != nil {
			t.Fatal(e)
		}
		if _, e = s.directorWorkspace(session); e != nil {
			t.Fatal(e)
		}
		for _, root := range []string{".agents", ".claude"} {
			data, e := os.ReadFile(filepath.Join(base, root, "skills", "studio-recovery", "SKILL.md"))
			if e != nil || string(data) != agents.StudioRecovery {
				t.Fatal(root, e)
			}
		}
	}
}

func TestRecoveryActivityUsesScopedStatusFacts(t *testing.T) {
	s := routerServer(t, nil)
	project, _ := s.database.GetActiveProjectID()
	op := &models.Op{Kind: "storyboard_audit", Status: "running", ProjectID: &project}
	if e := s.database.CreateOp(op); e != nil {
		t.Fatal(e)
	}
	if e := s.database.UpdateOp(op.ID, "succeeded", []byte(`{"status":"needs_attention","secret":"private-value"}`), nil); e != nil {
		t.Fatal(e)
	}
	private := "private-value may have been attempted"
	other := "other"
	data, _ := json.Marshal(s.chatActivity(project, []models.RenderJob{{ProjectID: &other, ID: "foreign-job"}, {ProjectID: &project, Error: &private}}))
	for _, expected := range []string{"needs_attention", "provider submission uncertain"} {
		if !strings.Contains(string(data), expected) {
			t.Fatal(string(data))
		}
	}
	for _, unexpected := range []string{"private-value", "foreign-job"} {
		if strings.Contains(string(data), unexpected) {
			t.Fatal(string(data))
		}
	}
}
