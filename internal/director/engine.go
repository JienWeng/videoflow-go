// Package director runs native local agents as planners. Application services execute approved work.
package director

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"time"
)

type Request struct{ Engine, Workspace, NativeID, Prompt string }
type Runner struct{ Codex, Claude string }

func SafeEnvironment(environment []string) []string {
	result := []string{}
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "FAL_KEY", "BYTEPLUS_API_KEY", "OPENROUTER_API_KEY", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN":
			continue
		}
		result = append(result, entry)
	}
	return result
}
func CodexArgs(native string) []string {
	args := []string{"--sandbox", "read-only", "--ask-for-approval", "never", "-c", "features.shell_tool=false", "-c", `web_search="disabled"`, "-c", "features.apps=false", "-c", "features.skill_mcp_dependency_install=false", "exec"}
	if native != "" {
		args = append(args, "resume", native)
	}
	return append(args, "--ignore-user-config", "--ignore-rules", "--json", "--skip-git-repo-check", "-")
}
func (r *Runner) Available(engine string) (string, error) {
	name := r.Codex
	if engine == "claude" {
		name = r.Claude
	} else if engine != "codex" {
		return "", fmt.Errorf("select codex or claude")
	}
	if name == "" {
		name = engine
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s CLI is not installed on the backend host; install and sign in there", engine)
	}
	return path, nil
}
func (r *Runner) Run(ctx context.Context, request Request, saveNative func(string) error) (json.RawMessage, error) {
	executable, err := r.Available(request.Engine)
	if err != nil {
		return nil, err
	}
	args := CodexArgs(request.NativeID)
	if request.Engine == "claude" {
		args = []string{"--restricted", "-p", "--output-format", "stream-json", "--verbose", "--tools", "", "--disallowedTools", "mcp__*", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--setting-sources", ""}
		if request.NativeID != "" {
			args = append(args, "--resume", request.NativeID)
		}
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = request.Workspace
	cmd.Env = SafeEnvironment(os.Environ())
	cmd.Stdin = strings.NewReader(request.Prompt)
	cmd.WaitDelay = 2 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	// Native stderr can contain private account details. Surface a bounded generic failure, not raw logs.
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", request.Engine, err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()
	scanner := bufio.NewScanner(io.LimitReader(stdout, 8<<20))
	scanner.Buffer(make([]byte, 64<<10), 2<<20)
	var result string
	native := request.NativeID
	completed := false
	for scanner.Scan() {
		var event struct {
			Type      string `json:"type"`
			ThreadID  string `json:"thread_id"`
			SessionID string `json:"session_id"`
			Subtype   string `json:"subtype"`
			IsError   bool   `json:"is_error"`
			Result    string `json:"result"`
			Item      struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil, fmt.Errorf("%s returned an invalid protocol event", request.Engine)
		}
		id := event.ThreadID
		if request.Engine == "claude" {
			id = event.SessionID
		}
		if id != "" {
			if len(id) > 160 || (native != "" && native != id) {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				return nil, fmt.Errorf("native session identity mismatch")
			}
			if native == "" {
				native = id
			}
			if err = saveNative(id); err != nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				return nil, err
			}
		}
		switch event.Type {
		case "item.completed":
			if event.Item.Type == "agent_message" {
				result = event.Item.Text
			}
		case "turn.completed":
			completed = true
		case "turn.failed", "error":
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil, fmt.Errorf("%s turn failed; check the native account/session before retrying", request.Engine)
		case "result":
			if event.IsError || event.Subtype != "success" {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				return nil, fmt.Errorf("Claude turn did not succeed")
			}
			result = event.Result
			completed = true
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if scanErr != nil || waitErr != nil {
		return nil, fmt.Errorf("%s process failed or protocol output exceeded limits; check CLI version/authentication", request.Engine)
	}
	if !completed || native == "" || strings.TrimSpace(result) == "" {
		return nil, fmt.Errorf("%s turn was interrupted or returned no result; it will not be replayed automatically", request.Engine)
	}
	result = strings.TrimSpace(result)
	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)
	if !json.Valid([]byte(result)) {
		return nil, fmt.Errorf("%s returned invalid structured JSON; review the session before retrying", request.Engine)
	}
	return json.RawMessage(result), nil
}

// Shape supplies field names and slice element shapes without coupling native agents to a provider SDK.
func Shape(t reflect.Type) any {
	if t == nil {
		return nil
	}
	if t.Kind() == reflect.Pointer {
		return Shape(t.Elem())
	}
	if t == reflect.TypeOf(json.RawMessage{}) {
		return map[string]any{}
	}
	switch t.Kind() {
	case reflect.Struct:
		fields := map[string]any{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			fields[name] = Shape(f.Type)
		}
		return fields
	case reflect.Array, reflect.Slice:
		return []any{Shape(t.Elem())}
	case reflect.Map:
		return map[string]any{}
	case reflect.String:
		return ""
	case reflect.Bool:
		return false
	default:
		return 0
	}
}
