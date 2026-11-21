package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"videoflow-go/internal/models"
)

func TestRepairChatContextPreservesOwnedIdentityAndNeedsNoReview(t *testing.T) {
	s, _ := bareServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("repair must never invoke AI") })
	project, _ := s.database.GetActiveProjectID()
	session, _ := s.database.CreateDirectorSession(project, "codex", "Story")
	c := &models.Character{ProjectID: &project, Name: "Mama"}
	s.database.CreateCharacter(c)
	raw, _ := json.Marshal(directorSelection{CharacterIDs: []string{"deleted-character", c.ID}, AssetIDs: []string{"deleted-asset"}})
	s.database.SetDirectorSelection(session.ID, project, raw)
	turn, e := s.database.BeginDirectorTurn(session.ID, project, "repair-test", "chat", json.RawMessage(`{}`))
	if e != nil {
		t.Fatal(e)
	}
	busy := call(t, s, "POST", "/director/sessions/"+session.ID+"/repair-context", `{}`)
	if busy.Code != 409 {
		t.Fatal("running session repaired", busy.Code)
	}
	s.database.FinishDirectorTurn(turn.ID, "failed", nil, "Interrupted")
	bad := call(t, s, "POST", "/chat", `{"message":"Plan shots"}`)
	if bad.Code != 409 || !strings.Contains(bad.Body.String(), "chat_context_invalid") {
		t.Fatal(bad.Code, bad.Body.String())
	}
	r := call(t, s, "POST", "/director/sessions/"+session.ID+"/repair-context", `{}`)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	saved, _ := s.database.GetDirectorSelection(session.ID)
	var selection directorSelection
	json.Unmarshal(saved, &selection)
	if len(selection.CharacterIDs) != 1 || selection.CharacterIDs[0] != c.ID || len(selection.AssetIDs) != 0 {
		t.Fatal(string(saved))
	}
	if ok, err := s.database.CompareDirectorSelection(session.ID, project, raw, json.RawMessage(`{}`)); err != nil || ok {
		t.Fatal("concurrent selection overwrite allowed", ok, err)
	}
	current, _ := s.database.GetDirectorSession(session.ID, project)
	if current.Status != "ready" {
		t.Fatal("a failed request left the session locked", current.Status)
	}
	r = call(t, s, "POST", "/director/sessions/"+session.ID+"/repair-context", `{}`)
	if r.Code != 200 {
		t.Fatal("repair not idempotent", r.Body.String())
	}
	// The failed request needs no review: the next message goes straight to the AI session.
	if next := call(t, s, "POST", "/chat", `{"message":"Plan shots"}`); strings.Contains(next.Body.String(), "review") {
		t.Fatal("chat still asks for a review", next.Code, next.Body.String())
	}
	other, _ := s.database.CreateProject("Other", "")
	s.database.ActivateProject(other.ID)
	r = call(t, s, "POST", "/director/sessions/"+session.ID+"/repair-context", `{}`)
	if r.Code != 404 {
		t.Fatal("cross-project repair allowed", r.Code)
	}
}
