package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"videoflow-go/internal/models"
)

func TestDashboardCanReadOlderUnresolvedOperations(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected provider call") })
	project, _ := s.database.GetActiveProjectID()
	old := models.Op{Kind: "video_generation", Status: "reconciliation_required", ProjectID: &project}
	if err := s.database.CreateOp(&old); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		op := models.Op{Kind: "assets", Status: "succeeded", ProjectID: &project}
		if err := s.database.CreateOp(&op); err != nil {
			t.Fatal(err)
		}
	}
	other, err := s.database.CreateProject("Other", "")
	if err != nil {
		t.Fatal(err)
	}
	foreign := models.Op{Kind: "assets", Status: "running", ProjectID: &other.ID}
	if err := s.database.CreateOp(&foreign); err != nil {
		t.Fatal(err)
	}
	response := call(t, s, "GET", "/ops?limit=all", "")
	var ops []models.Op
	if err := json.Unmarshal(response.Body.Bytes(), &ops); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, op := range ops {
		if op.ID == old.ID {
			found = true
		}
		if op.ID == foreign.ID {
			t.Fatal("foreign project operation leaked")
		}
	}
	if !found || len(ops) != 21 {
		t.Fatalf("dashboard missed older unresolved work: found=%v count=%d", found, len(ops))
	}
	response = call(t, s, "GET", "/ops", "")
	if err := json.Unmarshal(response.Body.Bytes(), &ops); err != nil {
		t.Fatal(err)
	}
	if len(ops) != 20 {
		t.Fatalf("default list limit changed: %d", len(ops))
	}
}
