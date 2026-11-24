package director

import (
	"strings"
	"testing"
)

func TestToolTurnsAreFreshSandboxedAndOffline(t *testing.T) {
	codex, _, err := turnArgs(Request{Engine: "codex", Tools: true, Prompt: "edit"})
	joined := strings.Join(codex, " ")
	if err != nil || !strings.Contains(joined, "--sandbox workspace-write") || !strings.Contains(joined, "sandbox_workspace_write.network_access=false") || !strings.Contains(joined, `web_search="disabled"`) || !strings.Contains(joined, "--ephemeral") || strings.Contains(joined, "resume") || strings.Contains(joined, "read-only") {
		t.Fatalf("%v %v", codex, err)
	}
	claude, _, err := turnArgs(Request{Engine: "claude", Tools: true, Prompt: "edit", Model: "opus", Effort: "low"})
	joined = strings.Join(claude, " ")
	if err != nil || !strings.Contains(joined, "--tools "+editorTools) || !strings.Contains(joined, "WebFetch,WebSearch,mcp__*") || !strings.Contains(joined, `"allowedDomains":[]`) || strings.Contains(joined, "--resume") || !strings.Contains(joined, "--model opus") {
		t.Fatalf("%v %v", claude, err)
	}
	for _, r := range []Request{{Engine: "codex", Tools: true, NativeID: "chat-session"}, {Engine: "codex", Tools: true, Images: []string{"a.png"}}, {Engine: "codex", Tools: true, ImageGeneration: true}} {
		if _, _, err = turnArgs(r); err == nil {
			t.Fatalf("a tool turn must be a fresh session: %+v", r)
		}
	}
}

func TestToolEnvironmentHoldsNoKeysOrSecrets(t *testing.T) {
	env := ToolEnvironment([]string{"PATH=/bin", "HOME=/home/v", "OPENAI_API_KEY=x", "ANTHROPIC_API_KEY=x", "GEMINI_API_KEY=x", "FAL_KEY=x", "GITHUB_TOKEN=x", "DB_PASSWORD=x", "AWS_REGION=x", "MY_SECRET=x", "VIDEOFLOW_EARSHOT_BIN=/opt/earshot/bin/earshot"})
	if strings.Join(env, " ") != "PATH=/bin HOME=/home/v VIDEOFLOW_EARSHOT_BIN=/opt/earshot/bin/earshot" {
		t.Fatal(env)
	}
}
