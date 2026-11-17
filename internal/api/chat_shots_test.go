package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
	"videoflow-go/internal/models"
)

func TestChatShotOperationNormalizesOrderAndPreservesWorkOnFailure(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected provider call", r.URL.Path) })
	project, _ := s.database.GetActiveProjectID()
	scene := &models.Scene{ProjectID: &project, Title: "Pawprints to Mama", Summary: "Pip follows pawprints to his mother", Duration: 5, AspectRatio: "9:16", CharacterIDsJSON: []byte(`[]`)}
	if err := s.database.CreateScene(scene); err != nil {
		t.Fatal(err)
	}
	raw := `{"shots":[{"duration":2,"prompt":"Shopkeeper points to pawprints"},{"duration":1.2,"prompt":"Pip follows the trail"},{"shot_order":3,"duration":1.8,"prompt":"Pip finds Mama"}]}`
	calls := 0
	s.agents.SetStructuredRunner(func(_ context.Context, _, _, _ string, target any) error {
		calls++
		return json.Unmarshal([]byte(raw), target)
	})
	run := func() *models.Op {
		t.Helper()
		r := call(t, s, "POST", "/scenes/"+scene.ID+"/shots/generate?background=true", `{}`)
		if r.Code != 202 {
			t.Fatal(r.Code, r.Body.String())
		}
		var reply struct {
			ID string `json:"op_id"`
		}
		json.Unmarshal(r.Body.Bytes(), &reply)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			op, _ := s.database.GetOp(reply.ID)
			if op.Status == "succeeded" || op.Status == "failed" {
				return op
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("shot operation did not settle")
		return nil
	}
	op := run()
	if op.Status != "succeeded" || calls != 1 {
		t.Fatal(op.Status, op.Error, calls)
	}
	shots, _ := s.database.ListShots(scene.ID)
	if len(shots) != 3 {
		t.Fatal(shots)
	}
	total := 0
	for i, shot := range shots {
		if shot.ShotOrder != i+1 || shot.Duration <= 0 || shot.Prompt == "" {
			t.Fatal(shot)
		}
		total += shot.Duration
	}
	if total != 5 {
		t.Fatal("wrong duration", total)
	}
	originalID := shots[0].ID
	raw = `{"shots":[{"duration":2,"prompt":""},{"duration":3,"prompt":"Pip finds Mama"}]}`
	op = run()
	if op.Status != "failed" || op.Error == nil || !strings.Contains(*op.Error, "Shot 1 is missing its visual prompt") {
		t.Fatal(op.Status, op.Error)
	}
	shots, _ = s.database.ListShots(scene.ID)
	if len(shots) != 3 || shots[0].ID != originalID {
		t.Fatal("failed retry discarded existing shots", shots)
	}
	raw = `{"shots":[{"duration":5,"prompt":"Pip and Mama reunite"}]}`
	if op = run(); op.Status != "succeeded" {
		t.Fatal("explicit retry did not recover", op.Error)
	}
	shots, _ = s.database.ListShots(scene.ID)
	if len(shots) != 1 || shots[0].ShotOrder != 1 || shots[0].Duration != 5 {
		t.Fatal(shots)
	}
}
