package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"videoflow-go/internal/models"
)

func TestLocalDirectorTextNeverFallsBackAndPersistsNativeSession(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("local text must not call OpenRouter") })
	s.cfg.UseLocalText = true
	fake := filepath.Join(t.TempDir(), "codex")
	os.WriteFile(fake, []byte(`#!/bin/sh
cat >/dev/null
printf '%s\n' '{"type":"thread.started","thread_id":"native-test"}' '{"type":"item.completed","item":{"type":"agent_message","text":"{\"options\":[{\"title\":\"A\",\"premise\":\"A premise\"},{\"title\":\"B\",\"premise\":\"B premise\"}],\"recommended_index\":0,\"reasoning\":\"ready\"}"}}' '{"type":"turn.completed"}'
`), 0700)
	s.directorRunner.Codex = fake
	response := call(t, s, "POST", "/director/sessions", `{"engine":"codex","title":"My film"}`)
	if response.Code != 201 {
		t.Fatalf("create %d %s", response.Code, response.Body.String())
	}
	response = call(t, s, "POST", "/ideas/develop", `{"idea":"A red car"}`)
	if response.Code != 200 {
		t.Fatalf("develop %d %s", response.Code, response.Body.String())
	}
	project, _ := s.database.GetActiveProjectID()
	sessions, _ := s.database.ListDirectorSessions(project)
	if len(sessions) != 1 || sessions[0].NativeID != "native-test" {
		t.Fatal(sessions)
	}
	turns, _ := s.database.ListDirectorTurns(sessions[0].ID, project)
	if len(turns) != 1 || turns[0].Status != "succeeded" {
		t.Fatal(turns)
	}
	s.directorRunner.Codex = "/does/not/exist"
	response = call(t, s, "POST", "/ideas/develop", `{"idea":"No fallback"}`)
	if response.Code == 200 || !strings.Contains(response.Body.String(), "not installed") {
		t.Fatal(response.Body.String())
	}
}
func TestDirectorProposalRejectsChangedReferencesBeforeExecution(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("must reject before provider") })
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	session, _ := s.database.CreateDirectorSession(project, "codex", "Identity test")
	path := filepath.Join(s.cfg.StorageRoot, "car.png")
	os.WriteFile(path, []byte("red car"), 0600)
	asset := &models.Asset{ProjectID: &project, Name: "Red car", Type: "prop", FilePath: path, MetadataJSON: []byte(`{"locked_traits":{"color":"red","type":"car"}}`)}
	s.database.CreateAsset(asset)
	selection := directorSelection{AssetIDs: []string{asset.ID}}
	snapshot, _ := s.buildDirectorContext(project, selection)
	raw, _ := json.Marshal(selection)
	proposal, _ := s.database.CreateDirectorProposal(session.ID, project, snapshot.Revision, raw)
	os.WriteFile(path, []byte("changed source"), 0600)
	r := call(t, s, "POST", "/ideas/develop?director_proposal="+proposal, `{"idea":"should not run"}`)
	if r.Code != 409 {
		t.Fatalf("%d %s", r.Code, r.Body.String())
	}
}
func TestDirectorCompositionPlannerUsesLocalSkillAndNeverExecutes(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("must not use remote text") })
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	s.database.CreateDirectorSession(project, "codex", "Editing")
	fake := filepath.Join(t.TempDir(), "codex")
	os.WriteFile(fake, []byte(`#!/bin/sh
cat >/dev/null
printf '%s\n' '{"type":"thread.started","thread_id":"native-edit"}' '{"type":"item.completed","item":{"type":"agent_message","text":"{\"version\":1,\"width\":640,\"height\":360,\"fps\":24,\"duration\":2,\"clips\":[],\"audio\":[],\"captions\":[]}"}}' '{"type":"turn.completed"}'
`), 0700)
	s.directorRunner.Codex = fake
	result := call(t, s, "POST", "/director/composition-plan", `{"instruction":"Make a simple two second composition","manifest":{"version":1,"width":640,"height":360,"fps":24,"duration":2,"clips":[],"audio":[],"captions":[]}}`)
	if result.Code != 200 || !strings.Contains(result.Body.String(), "manifest") {
		t.Fatalf("%d %s", result.Code, result.Body.String())
	}
	sessions, _ := s.database.ListDirectorSessions(project)
	turns, _ := s.database.ListDirectorTurns(sessions[0].ID, project)
	if len(turns) != 1 || !strings.Contains(string(turns[0].RequestJSON), "HyperFrames") {
		t.Fatal("reviewed skill context missing")
	}
}
func TestRequestProjectBindingSurvivesActiveProjectSwitch(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no inference") })
	original, _ := s.database.GetActiveProjectID()
	other, _ := s.database.CreateProject("Other", "")
	ctx := context.WithValue(context.Background(), directorBindingKey{}, directorBinding{ProjectID: original})
	s.database.ActivateProject(other.ID)
	captured, err := s.activeProjectID(ctx)
	if err != nil || captured != original {
		t.Fatalf("request changed project %s %v", captured, err)
	}
}

func TestIdentityEditDuringNativeTurnRejectsOldContextProposal(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no remote text") })
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	session, _ := s.database.CreateDirectorSession(project, "codex", "Race test")
	path := filepath.Join(s.cfg.StorageRoot, "reference.png")
	os.WriteFile(path, []byte("old identity reference"), 0600)
	asset := &models.Asset{ProjectID: &project, Name: "Car", Type: "prop", FilePath: path}
	s.database.CreateAsset(asset)
	selection, _ := json.Marshal(directorSelection{AssetIDs: []string{asset.ID}})
	s.database.SetDirectorSelection(session.ID, project, selection)
	fake := filepath.Join(t.TempDir(), "codex")
	program := fmt.Sprintf(`#!/usr/bin/env python3
import sys,json
sys.stdin.read()
open(%q,'wb').write(b'changed identity during planning')
print(json.dumps({'type':'thread.started','thread_id':'native-race'}))
print(json.dumps({'type':'item.completed','item':{'type':'agent_message','text':json.dumps({'reply':'Ready','action':'generate_script','scene_count':1})}}))
print(json.dumps({'type':'turn.completed'}))
`, path)
	os.WriteFile(fake, []byte(program), 0700)
	s.directorRunner.Codex = fake
	response := call(t, s, "POST", "/chat", `{"message":"Plan the red car story"}`)
	if response.Code != 409 {
		t.Fatalf("stale turn accepted: %d %s", response.Code, response.Body.String())
	}
}
