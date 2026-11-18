package director

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeCLI(t *testing.T, name, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func waitFor(t *testing.T, m *AuthManager, engine, status string) AuthState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s := m.Status(context.Background(), engine)
		if s.Login != nil && s.Login.Status == status {
			return s
		}
		time.Sleep(50 * time.Millisecond)
	}
	s := m.Status(context.Background(), engine)
	t.Fatalf("want %s, got %+v", status, s.Login)
	return s
}

func TestCodexDeviceLoginShowsLinkAndCode(t *testing.T) {
	codex := fakeCLI(t, "codex", `
if [ "$1 $2" = "login status" ]; then [ -f "$0.done" ] && { echo "Logged in using ChatGPT"; exit 0; }; echo "Not logged in"; exit 1; fi
printf '\033[94mhttps://auth.openai.com/codex/device\033[0m\n   \033[94mABCD-12345\033[0m\n'
sleep 0.3; touch "$0.done"`)
	m := NewAuthManager(&Runner{Codex: codex})
	if m.Status(context.Background(), "codex").SignedIn {
		t.Fatal("should start signed out")
	}
	login, err := m.Start("codex")
	if err != nil || login.Status != "waiting" || login.URL != "https://auth.openai.com/codex/device" || login.Code != "ABCD-12345" || login.NeedsCode {
		t.Fatalf("%+v %v", login, err)
	}
	s := waitFor(t, m, "codex", "complete")
	if !s.SignedIn || s.Method != "ChatGPT" || s.Login.Code != "" {
		t.Fatalf("%+v %+v", s, s.Login)
	}
}

func TestClaudeLoginAcceptsPastedCode(t *testing.T) {
	claude := fakeCLI(t, "claude", `
if [ "$1 $2" = "auth status" ]; then [ -f "$0.done" ] && echo '{"loggedIn":true,"authMethod":"claude.ai"}' || echo '{"loggedIn":false}'; exit 0; fi
echo "If the browser didn't open, visit: https://claude.com/cai/oauth/authorize?code=true&state=x"
printf 'Paste code here if prompted > '
read code; [ "$code" = "good#state" ] || { echo "Invalid code"; exit 1; }; touch "$0.done"`)
	m := NewAuthManager(&Runner{Claude: claude})
	login, err := m.Start("claude")
	if err != nil || login.Status != "waiting" || !login.NeedsCode || !strings.HasPrefix(login.URL, "https://claude.com/cai/oauth/authorize") {
		t.Fatalf("%+v %v", login, err)
	}
	if err = m.SubmitCode("claude", "bad\ninjected"); err == nil {
		t.Fatal("multi-line code accepted")
	}
	if err = m.SubmitCode("claude", "  good#state "); err != nil {
		t.Fatal(err)
	}
	s := waitFor(t, m, "claude", "complete")
	if !s.SignedIn || s.Method != "Claude subscription" {
		t.Fatalf("%+v", s)
	}
}

func TestClaudeLoginReportsRejectedCodeWithoutEchoingIt(t *testing.T) {
	claude := fakeCLI(t, "claude", `
if [ "$1 $2" = "auth status" ]; then echo '{"loggedIn":false}'; exit 0; fi
echo "visit: https://claude.com/cai/oauth/authorize?state=secret-state"
read code; echo "Invalid code: $code"; exit 1`)
	m := NewAuthManager(&Runner{Claude: claude})
	if _, err := m.Start("claude"); err != nil {
		t.Fatal(err)
	}
	if err := m.SubmitCode("claude", "private-code"); err != nil {
		t.Fatal(err)
	}
	s := waitFor(t, m, "claude", "failed")
	if strings.Contains(s.Login.Message, "private-code") || strings.Contains(s.Login.Message, "secret-state") || s.Login.URL != "" {
		t.Fatalf("leaked: %+v", s.Login)
	}
}
