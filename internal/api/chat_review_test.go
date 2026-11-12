package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"videoflow-go/internal/models"
)

func reviewServer(t *testing.T) *Server {
	t.Helper()
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("review must never call a remote provider") })
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	session, err := s.database.CreateDirectorSession(project, "codex", "Review")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.database.SetDirectorNativeID(session.ID, "parent-native"); err != nil {
		t.Fatal(err)
	}
	return s
}
func reviewCLI(t *testing.T, s *Server, extra string) string {
	t.Helper()
	log := filepath.Join(t.TempDir(), "children")
	fake := filepath.Join(t.TempDir(), "codex")
	script := fmt.Sprintf(`#!/bin/sh
cat >/dev/null
printf '%%s\n' "$PWD|$*" >> %q
%s
printf '%%s\n' '{"type":"thread.started","thread_id":"private-child"}' '{"type":"item.completed","item":{"type":"agent_message","text":"{\"summary\":\"Review assessment\",\"suggestions\":[\"Clarify motivation\"]}"}}' '{"type":"turn.completed"}'
`, log, extra)
	if err := os.WriteFile(fake, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	s.directorRunner.Codex = fake
	return log
}
func TestChatReviewIsolatedBoundedChildrenAndReplay(t *testing.T) {
	s := reviewServer(t)
	// A barrier proves both children run concurrently (sequential execution would time out).
	barrier := filepath.Join(t.TempDir(), "barrier")
	log := reviewCLI(t, s, fmt.Sprintf(`mkdir -p %q
case "$PWD" in *review-story-*) touch %q;; *) touch %q;; esac
n=0
while [ ! -f %q ] || [ ! -f %q ]; do n=$((n+1)); [ "$n" -lt 100 ] || exit 2; sleep 0.02; done`, barrier, barrier+"/story", barrier+"/continuity", barrier+"/story", barrier+"/continuity"))
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("POST", "/chat/review", strings.NewReader(`{"message":"Review my story"}`))
		r.Header.Set("Idempotency-Key", "review-once")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		var result specialistReview
		if json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Subagents) != 2 || result.Subagents[0].Status != "succeeded" || result.Subagents[1].Status != "succeeded" {
			t.Fatal(w.Body.String())
		}
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected exactly two children: %s", raw)
	}
	first := strings.SplitN(lines[0], "|", 2)[0]
	second := strings.SplitN(lines[1], "|", 2)[0]
	if first == second {
		t.Fatal("children shared workspace")
	}
	for _, line := range lines {
		if strings.Contains(line, "resume") || !strings.Contains(line, "--sandbox read-only") || !strings.Contains(line, "features.shell_tool=false") {
			t.Fatal(line)
		}
	}
	for _, dir := range []string{first, second} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("private child workspace retained")
		}
	}
	project, _ := s.database.GetActiveProjectID()
	sessions, _ := s.database.ListDirectorSessions(project)
	if len(sessions) != 1 || sessions[0].NativeID != "parent-native" {
		t.Fatal(sessions)
	}
	turns, _ := s.database.ListDirectorTurns(sessions[0].ID, project)
	if len(turns) != 1 || turns[0].Kind != "specialist_review" || turns[0].Status != "succeeded" || !strings.Contains(string(turns[0].RequestJSON), `"user_message":"Review my story"`) {
		t.Fatal(turns)
	}
}
func TestChatReviewCapabilitiesAndUnavailableSession(t *testing.T) {
	s := reviewServer(t)
	s.directorRunner.Codex = "/missing-review-cli"
	w := call(t, s, "GET", "/chat/capabilities", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"supported":false`) || !strings.Contains(w.Body.String(), `"terminal":false`) {
		t.Fatal(w.Body.String())
	}
	w = call(t, s, "POST", "/chat/review", `{"message":"review"}`)
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	project, _ := s.database.GetActiveProjectID()
	sessions, _ := s.database.ListDirectorSessions(project)
	turns, _ := s.database.ListDirectorTurns(sessions[0].ID, project)
	if len(turns) != 0 {
		t.Fatal(turns)
	}
	reviewCLI(t, s, "")
	w = call(t, s, "GET", "/chat/capabilities", "")
	if !strings.Contains(w.Body.String(), `"supported":true`) {
		t.Fatal(w.Body.String())
	}
	s.cfg.UseLocalText = false
	if w = call(t, s, "POST", "/chat/review", `{"message":"review"}`); w.Code != 409 {
		t.Fatal(w.Code)
	}
}
func TestChatReviewPartialFailureAndContextMutationRecorded(t *testing.T) {
	for _, mutate := range []bool{false, true} {
		t.Run(fmt.Sprint(mutate), func(t *testing.T) {
			s := reviewServer(t)
			project, _ := s.database.GetActiveProjectID()
			file := filepath.Join(s.cfg.StorageRoot, "review-reference.png")
			os.WriteFile(file, []byte("old"), 0600)
			asset := &models.Asset{ProjectID: &project, Name: "Reference", Type: "prop", FilePath: file}
			s.database.CreateAsset(asset)
			sessions, _ := s.database.ListDirectorSessions(project)
			selected, _ := json.Marshal(directorSelection{AssetIDs: []string{asset.ID}})
			s.database.SetDirectorSelection(sessions[0].ID, project, selected)
			extra := `case "$PWD" in *review-story-*) exit 2;; esac`
			if mutate {
				extra = fmt.Sprintf("printf changed > %q", file)
			}
			reviewCLI(t, s, extra)
			w := call(t, s, "POST", "/chat/review", `{"message":"review"}`)
			expected := 200
			if mutate {
				expected = 409
			}
			if w.Code != expected {
				t.Fatal(w.Code, w.Body.String())
			}
			turns, _ := s.database.ListDirectorTurns(sessions[0].ID, project)
			if len(turns) != 1 || turns[0].Status != "failed" {
				t.Fatal(turns)
			}
			var result specialistReview
			json.Unmarshal(turns[0].ResultJSON, &result)
			if mutate {
				for _, child := range result.Subagents {
					if child.Status != "failed" || child.Summary != "" {
						t.Fatal(child)
					}
				}
			} else if result.Subagents[0].Status != "failed" || result.Subagents[1].Status != "succeeded" {
				t.Fatal(result)
			}
		})
	}
}
func TestChatReviewRejectsCrossProjectSelectionAndInputBounds(t *testing.T) {
	s := reviewServer(t)
	log := reviewCLI(t, s, "")
	other, _ := s.database.CreateProject("Other", "")
	asset := &models.Asset{ProjectID: &other.ID, Name: "Foreign", Type: "prop"}
	s.database.CreateAsset(asset)
	w := call(t, s, "POST", "/chat/review", fmt.Sprintf(`{"message":"review","selected":{"kind":"asset","id":%q}}`, asset.ID))
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call(t, s, "POST", "/chat/review", `{"message":"`+strings.Repeat("x", 16001)+`"}`)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("CLI started for rejected input")
	}
}

func TestChatReviewCancelAndSessionLock(t *testing.T) {
	s := reviewServer(t)
	log := reviewCLI(t, s, "while :; do sleep 0.02; done")
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, httptest.NewRequest("POST", "/chat/review", strings.NewReader(`{"message":"review"}`)))
		done <- w
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		raw, _ := os.ReadFile(log)
		if strings.Count(string(raw), "\n") == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("children did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	capability := call(t, s, "GET", "/chat/capabilities", "")
	if !strings.Contains(capability.Body.String(), `"supported":false`) || !strings.Contains(capability.Body.String(), "busy") {
		t.Fatal(capability.Body.String())
	}
	w := call(t, s, "POST", "/chat/review", `{"message":"overlapping"}`)
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	project, _ := s.database.GetActiveProjectID()
	sessions, _ := s.database.ListDirectorSessions(project)
	w = call(t, s, "POST", "/director/sessions/"+sessions[0].ID+"/cancel", `{}`)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	select {
	case response := <-done:
		if response.Code != 409 {
			t.Fatal(response.Code, response.Body.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("review ignored cancellation")
	}
	turns, _ := s.database.ListDirectorTurns(sessions[0].ID, project)
	if len(turns) != 1 || turns[0].Status != "interrupted" || !strings.Contains(string(turns[0].ResultJSON), `"status":"failed"`) {
		t.Fatal(turns)
	}
}
func TestChatReviewRequiresSelectedSession(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no provider") })
	s.cfg.UseLocalText = true
	reviewCLI(t, s, "")
	w := call(t, s, "POST", "/chat/review", `{"message":"review"}`)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "select a local") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestChatReviewPartialFailureRequiresAcknowledgement(t *testing.T) {
	s := reviewServer(t)
	log := reviewCLI(t, s, `case "$PWD" in *review-story-*) exit 2;; esac`)
	w := call(t, s, "POST", "/chat/review", `{"message":"review"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"failed"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call(t, s, "GET", "/chat/capabilities", "")
	if !strings.Contains(w.Body.String(), `"supported":false`) || !strings.Contains(w.Body.String(), "requires review") {
		t.Fatal(w.Body.String())
	}
	w = call(t, s, "POST", "/chat/review", `{"message":"do not start"}`)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "acknowledge") {
		t.Fatal(w.Code, w.Body.String())
	}
	raw, _ := os.ReadFile(log)
	if strings.Count(string(raw), "\n") != 2 {
		t.Fatal("unexpected retry", string(raw))
	}
	project, _ := s.database.GetActiveProjectID()
	sessions, _ := s.database.ListDirectorSessions(project)
	w = call(t, s, "POST", "/director/sessions/"+sessions[0].ID+"/acknowledge", `{}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call(t, s, "GET", "/chat/capabilities", "")
	if !strings.Contains(w.Body.String(), `"supported":true`) {
		t.Fatal(w.Body.String())
	}
}

func TestChatReviewGlobalNarrativeReachesBothChildrenAndMutationInvalidates(t *testing.T) {
	for _, mutate := range []bool{false, true} {
		t.Run(fmt.Sprint(mutate), func(t *testing.T) {
			s := reviewServer(t)
			project, _ := s.database.GetActiveProjectID()
			script := &models.Script{ProjectID: &project, Idea: "A lighthouse keeper", Title: "Light Returns", Summary: "Keeper confronts the storm", DraftJSON: []byte(`{"dialogue":"Keep the light burning"}`)}
			if err := s.database.CreateScript(script); err != nil {
				t.Fatal(err)
			}
			scene := &models.Scene{ProjectID: &project, ScriptID: &script.ID, Title: "Storm approaches", Summary: "Keeper climbs the tower", SceneJSON: []byte(`{"action":"Raise the beacon"}`)}
			if err := s.database.CreateScene(scene); err != nil {
				t.Fatal(err)
			}
			shot := &models.Shot{SceneID: scene.ID, Prompt: "Closeup of keeper lighting the beacon", ShotJSON: []byte(`{"dialogue":"Ships depend on us"}`)}
			if err := s.database.CreateShot(shot); err != nil {
				t.Fatal(err)
			}
			promptLog := filepath.Join(t.TempDir(), "prompts")
			fakeLog := reviewCLI(t, s, fmt.Sprintf(`printf '%%s\n' "$input" >> %q`, promptLog))
			fakePath := s.directorRunner.Codex
			program, _ := os.ReadFile(fakePath)
			program = []byte(strings.Replace(string(program), "cat >/dev/null", "input=$(cat)", 1))
			release := filepath.Join(t.TempDir(), "release")
			if mutate {
				program = []byte(strings.Replace(string(program), "printf '%s\\n' '{\"type\":\"thread.started\"", fmt.Sprintf("while [ ! -f %q ]; do sleep 0.02; done\nprintf '%%s\\n' '{\"type\":\"thread.started\"", release), 1))
			}
			if err := os.WriteFile(fakePath, program, 0700); err != nil {
				t.Fatal(err)
			}
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				w := httptest.NewRecorder()
				s.router.ServeHTTP(w, httptest.NewRequest("POST", "/chat/review", strings.NewReader(`{"message":"Review my story"}`)))
				done <- w
			}()
			if mutate {
				deadline := time.Now().Add(5 * time.Second)
				for {
					raw, _ := os.ReadFile(fakeLog)
					if strings.Count(string(raw), "\n") == 2 {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("children did not start")
					}
					time.Sleep(10 * time.Millisecond)
				}
				updatedScene, loadErr := s.database.GetScene(scene.ID)
				scene = updatedScene
				if loadErr != nil {
					t.Fatal(loadErr)
				}
				scene.Summary = "Keeper abandons the tower"
				if err := s.database.UpdateScene(scene); err != nil {
					t.Fatal(err)
				}
				os.WriteFile(release, []byte("ready"), 0600)
			}
			select {
			case w := <-done:
				want := 200
				if mutate {
					want = 409
				}
				if w.Code != want {
					t.Fatal(w.Code, w.Body.String())
				}
			case <-time.After(5 * time.Second):
				t.Fatal("review did not finish")
			}
			raw, err := os.ReadFile(promptLog)
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range []string{"Storm approaches", "Keeper climbs the tower", "Closeup of keeper lighting the beacon", "Ships depend on us", "Light Returns", "Keep the light burning"} {
				if strings.Count(string(raw), value) != 2 {
					t.Fatalf("both children must receive %q: %s", value, raw)
				}
			}
			if mutate {
				sessions, _ := s.database.ListDirectorSessions(project)
				turns, _ := s.database.ListDirectorTurns(sessions[0].ID, project)
				if len(turns) != 1 || turns[0].Status != "failed" {
					t.Fatal(turns)
				}
			}
		})
	}
}
func TestChatReviewRejectsOversizedNarrativeBeforeStarting(t *testing.T) {
	s := reviewServer(t)
	log := reviewCLI(t, s, "")
	project, _ := s.database.GetActiveProjectID()
	script := &models.Script{ProjectID: &project, Title: "Large story", Summary: strings.Repeat("x", 256<<10), DraftJSON: []byte(`{}`)}
	if err := s.database.CreateScript(script); err != nil {
		t.Fatal(err)
	}
	w := call(t, s, "POST", "/chat/review", `{"message":"Review my story"}`)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "256 KiB") {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("oversized story started CLI")
	}
}

func TestLocalMainChatCancellationRecordsInterruptedTurn(t *testing.T) {
	s := reviewServer(t)
	log := reviewCLI(t, s, "while :; do sleep 0.02; done")
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, httptest.NewRequest("POST", "/chat", strings.NewReader(`{"message":"Plan the scene"}`)))
		done <- w
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		raw, _ := os.ReadFile(log)
		if len(raw) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("main planner did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	project, _ := s.database.GetActiveProjectID()
	sessions, _ := s.database.ListDirectorSessions(project)
	w := call(t, s, "POST", "/director/sessions/"+sessions[0].ID+"/cancel", `{}`)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	select {
	case response := <-done:
		if response.Code == 200 {
			t.Fatal("canceled planning succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("main planner ignored cancellation")
	}
	turns, _ := s.database.ListDirectorTurns(sessions[0].ID, project)
	if len(turns) != 1 || turns[0].Status != "interrupted" {
		t.Fatal(turns)
	}
	if _, ok := s.directorCancels.Load(sessions[0].ID); ok {
		t.Fatal("completed turn retained cancel handle")
	}
}
