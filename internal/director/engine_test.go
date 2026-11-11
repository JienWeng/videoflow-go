package director

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexResumesExactSessionAndRequiresCompletion(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "codex")
	script := `#!/bin/sh
cat > /dev/null
printf '%s\n' '{"type":"thread.started","thread_id":"native-123"}' '{"type":"item.completed","item":{"type":"agent_message","text":"{\"reply\":\"hello\"}"}}' '{"type":"turn.completed"}'
`
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	runner := Runner{Codex: executable}
	var native string
	result, err := runner.Run(context.Background(), Request{Engine: "codex", Workspace: dir, NativeID: "native-123", Prompt: "test"}, func(id string) error { native = id; return nil })
	if err != nil || native != "native-123" || !strings.Contains(string(result), "hello") {
		t.Fatalf("result %s native %s err %v", result, native, err)
	}
	args := CodexArgs("native-123")
	if strings.Contains(strings.Join(args, " "), "--last") || !strings.Contains(strings.Join(args, " "), "resume native-123") {
		t.Fatal(args)
	}
	os.WriteFile(executable, []byte("#!/bin/sh\ncat >/dev/null\nprintf '%s\\n' '{\"type\":\"thread.started\",\"thread_id\":\"native-123\"}'\n"), 0700)
	if _, err = runner.Run(context.Background(), Request{Engine: "codex", Workspace: dir, Prompt: "test"}, func(string) error { return nil }); err == nil {
		t.Fatal("truncated turn accepted")
	}
}
func TestClaudeRejectsFailureAndWrongNativeSession(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "claude")
	os.WriteFile(executable, []byte("#!/bin/sh\ncat >/dev/null\nprintf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"session_id\":\"wrong\",\"result\":\"{}\"}'\n"), 0700)
	_, err := (&Runner{Claude: executable}).Run(context.Background(), Request{Engine: "claude", Workspace: dir, NativeID: "expected", Prompt: "hello"}, func(string) error { return nil })
	if err == nil {
		t.Fatal("wrong session accepted")
	}
}
func TestNativeEnvironmentRemovesMediaCredentials(t *testing.T) {
	env := SafeEnvironment([]string{"HOME=/tmp/home", "PATH=/bin", "FAL_KEY=secret", "OPENROUTER_API_KEY=secret", "BYTEPLUS_API_KEY=secret", "ANTHROPIC_API_KEY=auth", "CODEX_HOME=/tmp/codex"})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "FAL_KEY") || strings.Contains(joined, "OPENROUTER") || strings.Contains(joined, "BYTEPLUS") {
		t.Fatal("provider keys inherited")
	}
	if !strings.Contains(joined, "ANTHROPIC_API_KEY=auth") {
		t.Fatal("native auth removed")
	}
}
