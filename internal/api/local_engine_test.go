package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// All planning and writing goes through a local Codex or Claude Code session. Tests never
// start the real CLIs: the test binary itself stands in for Codex and passes each turn to
// the test's own fake text service, so a test scripts the planner's answers there.
const fakeEngineURL = "VIDEOFLOW_TEST_ENGINE_URL"

func TestMain(m *testing.M) {
	if url := os.Getenv(fakeEngineURL); url != "" {
		os.Exit(fakeEngineTurn(url, os.Args[1:]))
	}
	// Tests never run earshot: a test that needs a music bed makes its own.
	makeMusicBed = func(context.Context, string, int, string) error {
		return fmt.Errorf("earshot is not run in tests")
	}
	os.Exit(m.Run())
}

// fakeEngineTurn answers one "codex exec" turn. Any other command (such as the checks for
// sign-in and picture making) prints nothing, which reads as signed in, without pictures.
func fakeEngineTurn(url string, args []string) int {
	turn := false
	for _, arg := range args {
		turn = turn || arg == "exec"
	}
	if !turn {
		return 0
	}
	raw, _ := io.ReadAll(os.Stdin)
	system, user := string(raw), string(raw)
	if at := strings.Index(system, "\nSelected project context: "); at >= 0 {
		if cut := strings.Index(system[at:], "\nRequest: "); cut >= 0 {
			system, user = system[:at+cut], system[at+cut+len("\nRequest: "):]
		}
	}
	// Attached pictures reach the fake service as data URLs, the way a real session sees them.
	for i, arg := range args {
		path := strings.TrimPrefix(arg, "--image=")
		if arg == "--image" && i+1 < len(args) {
			path = args[i+1]
		} else if path == arg {
			continue
		}
		if data, err := os.ReadFile(path); err == nil {
			user += "\nattached image: data:" + http.DetectContentType(data) + ";base64," + base64.StdEncoding.EncodeToString(data)
		}
	}
	body, _ := json.Marshal(map[string]any{"model": "local-session", "messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}}})
	request, _ := http.NewRequest("POST", url+"/chat/completions", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 1
	}
	defer response.Body.Close()
	var answer struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if response.StatusCode >= 400 || json.NewDecoder(response.Body).Decode(&answer) != nil || len(answer.Choices) == 0 {
		fmt.Println(`{"type":"turn.failed"}`)
		return 0
	}
	for _, event := range []map[string]any{
		{"type": "thread.started", "thread_id": "test-planner"},
		{"type": "item.completed", "item": map[string]string{"type": "agent_message", "text": answer.Choices[0].Message.Content}},
		{"type": "turn.completed"},
	} {
		line, _ := json.Marshal(event)
		fmt.Println(string(line))
	}
	return 0
}

// useFakeEngine selects a local session in the active project whose turns are answered by
// the fake text service at url (its /chat/completions), and keeps the real CLIs out of reach.
func useFakeEngine(t *testing.T, s *Server, url string) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "codex")
	script := fmt.Sprintf("#!/bin/sh\n%s=%q exec %q \"$@\"\n", fakeEngineURL, url, binary)
	if err = os.WriteFile(fake, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	s.directorRunner.Codex = fake
	s.directorRunner.Claude = filepath.Join(dir, "claude-is-not-installed")
	project, err := s.database.GetActiveProjectID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.database.CreateDirectorSession(project, "codex", "Test planner"); err != nil {
		t.Fatal(err)
	}
}

// plannerContext is the context of a request made with the selected local session.
func plannerContext(t *testing.T, s *Server) context.Context {
	t.Helper()
	project, _ := s.database.GetActiveProjectID()
	sessions, _ := s.database.ListDirectorSessions(project)
	for _, session := range sessions {
		if session.Selected {
			return context.WithValue(context.Background(), directorBindingKey{}, directorBinding{ProjectID: project, SessionID: session.ID})
		}
	}
	t.Fatal("no local session is selected")
	return nil
}

// plannedAssetsAnswer is what a fake planner says when asked which props and places a
// scene needs: none.
const plannedAssetsAnswer = `{"assets":[]}`

func asksForPlannedAssets(system string) bool {
	return strings.Contains(system, "Plan only essential visible props")
}

// awaitReadyStory waits until a guided story is ready for review, with no video requested
// yet, and returns its scenes.
func awaitReadyStory(t *testing.T, s *Server, opID string) []string {
	t.Helper()
	var ready struct {
		Status string   `json:"status"`
		Scenes []string `json:"scene_ids"`
	}
	deadline := time.Now().Add(10 * time.Second)
	for ready.Status != "ready_to_render" {
		if time.Now().After(deadline) {
			t.Fatal("guided story did not get ready")
		}
		op, err := s.database.GetOp(opID)
		if err != nil {
			t.Fatal(err)
		}
		if op.Status == "failed" {
			t.Fatal("guided story failed: ", *op.Error)
		}
		if op.Status == "succeeded" {
			if json.Unmarshal(op.ResultJSON, &ready) != nil || ready.Status != "ready_to_render" {
				t.Fatal(string(op.ResultJSON))
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	project, _ := s.database.GetActiveProjectID()
	if jobs, _ := s.database.ListRenderJobs(project); len(jobs) != 0 {
		t.Fatal("a video was requested before the story was reviewed")
	}
	return ready.Scenes
}

// confirmedSceneRender is the request the owner sends for a scene's video after reading
// its render plan: the plan as displayed, with its paid calls confirmed.
func confirmedSceneRender(t *testing.T, s *Server, scene string) string {
	t.Helper()
	w := call(t, s, "GET", "/scenes/"+scene+"/production", "")
	var plan productionPlan
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil || plan.ID == "" {
		t.Fatal(w.Code, w.Body.String())
	}
	spend := productionSpend(plan, false)
	return string(voiceJSON(map[string]any{"confirmed_plan_id": plan.ID, "confirmed_calls": spend.Calls, "confirmed_seconds": spend.Seconds, "confirmed_storyboard_calls": spend.Storyboards}))
}

// requestSceneVideo asks for one scene's video the way the owner does after review, and
// returns the video requests it made.
func requestSceneVideo(t *testing.T, s *Server, scene string) []string {
	t.Helper()
	w := call(t, s, "POST", "/scenes/"+scene+"/render", confirmedSceneRender(t, s, scene))
	var started struct {
		IDs []string `json:"job_ids"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &started) != nil || len(started.IDs) == 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	return started.IDs
}

// renderReadyStory waits for a guided story, then asks for each scene's video the way the
// owner does after review, and returns the video requests.
func renderReadyStory(t *testing.T, s *Server, opID string) []string {
	t.Helper()
	ids := []string{}
	for _, scene := range awaitReadyStory(t, s, opID) {
		ids = append(ids, requestSceneVideo(t, s, scene)...)
	}
	return ids
}
