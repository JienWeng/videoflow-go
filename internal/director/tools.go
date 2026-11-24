package director

import (
	"fmt"
	"strings"
)

// A tool turn is a fresh, one-off session that may run commands inside its own
// workspace: the AI editor renders title cards and makes sounds there. It never
// resumes a chat session, has no web access and no network for its commands, and
// its environment holds no service keys.

// editorTools are the only tools a Claude Code tool turn gets.
const editorTools = "Bash,Read,Write,Edit,Glob,Grep"

// claudeSandbox keeps Bash in Claude Code's sandbox: writes stay in the workspace and
// commands get no network (no domain is allowed).
const claudeSandbox = `{"sandbox":{"enabled":true,"autoAllowBashIfSandboxed":true,"allowUnsandboxedCommands":false,"network":{"allowedDomains":[]}},"permissions":{"deny":["WebFetch","WebSearch"]}}`

// toolArgs builds a tool turn's CLI arguments.
func toolArgs(request Request) ([]string, error) {
	if request.NativeID != "" || request.ImageGeneration || len(request.Images) > 0 {
		return nil, fmt.Errorf("a tool turn is a fresh session with no images")
	}
	if request.Engine == "claude" {
		return []string{"-p", "--output-format", "stream-json", "--verbose", "--tools", editorTools, "--allowedTools", editorTools, "--disallowedTools", "WebFetch,WebSearch,mcp__*", "--permission-mode", "acceptEdits", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--setting-sources", "", "--settings", claudeSandbox}, nil
	}
	// Codex's workspace-write sandbox lets commands write only in the workspace and
	// keeps their network off.
	return []string{"--sandbox", "workspace-write", "--ask-for-approval", "never", "-c", "sandbox_workspace_write.network_access=false", "-c", "features.image_generation=false", "-c", `web_search="disabled"`, "-c", "features.apps=false", "-c", "features.skill_mcp_dependency_install=false", "exec", "--ephemeral", "--ignore-user-config", "--ignore-rules", "--json", "--skip-git-repo-check", "-"}, nil
}

// ToolEnvironment is the environment of a tool turn: like SafeEnvironment, and also
// without anything that looks like a key, token, secret or password, because the
// session's commands can read their environment. The CLI's own sign-in lives in its
// home folder, not here.
func ToolEnvironment(environment []string) []string {
	result := []string{}
	for _, entry := range SafeEnvironment(environment) {
		name, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(name)
		if strings.HasSuffix(upper, "_KEY") || strings.Contains(upper, "API_KEY") || strings.Contains(upper, "TOKEN") || strings.Contains(upper, "SECRET") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "CREDENTIAL") || strings.HasPrefix(upper, "AWS_") {
			continue
		}
		result = append(result, entry)
	}
	return result
}
