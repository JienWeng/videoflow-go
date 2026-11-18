package api

import (
	"testing"
	"videoflow-go/internal/models"
)

func TestDeleteProjectMaintainsWorkspace(t *testing.T) {
	s := routerServer(t, nil)
	original, err := s.database.GetActiveProjectID()
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.database.CreateProject("Other", "")
	if err != nil {
		t.Fatal(err)
	}
	op := &models.Op{Kind: "storyboard", Status: "running", ProjectID: &original}
	if err = s.database.CreateOp(op); err != nil {
		t.Fatal(err)
	}
	remove := func(id string, want int) {
		t.Helper()
		r := call(t, s, "DELETE", "/projects/"+id, "")
		if r.Code != want {
			t.Fatalf("%d: %s", r.Code, r.Body.String())
		}
	}
	remove(original, 409)
	if err = s.database.UpdateOp(op.ID, "succeeded", []byte("{}"), nil); err != nil {
		t.Fatal(err)
	}
	remove(original, 200)
	if id, err := s.database.GetActiveProjectID(); err != nil || id != other.ID {
		t.Fatal(id, err)
	}
	if _, err := s.database.GetOp(op.ID); err == nil {
		t.Fatal("deleted project left operation history")
	}
	remove(original, 404)
	remove(other.ID, 200)
	projects, err := s.database.ListProjects()
	if err != nil || len(projects) != 1 || !projects[0].IsActive || projects[0].ID == other.ID {
		t.Fatal(projects, err)
	}
}
