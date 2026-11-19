// Package director runs native local agents as planners. Application services execute approved work.
package director

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

type Request struct {
	Engine, Workspace, NativeID, Prompt string
	// Model and Effort are optional overrides; empty keeps the tool's default.
	Model, Effort   string
	ImageGeneration bool
	Images          []string
}
type Runner struct{ Codex, Claude string }

func SafeEnvironment(environment []string) []string {
	result := []string{}
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "OPENAI_API_KEY", "OPENAI_BASE_URL", "ELEVENLABS_API_KEY", "FISH_API_KEY", "FAL_KEY", "BYTEPLUS_API_KEY", "OPENROUTER_API_KEY", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN":
			continue
		}
		result = append(result, entry)
	}
	return result
}
func CodexArgs(native string) []string {
	args := []string{"--sandbox", "read-only", "--ask-for-approval", "never", "-c", "features.shell_tool=false", "-c", "features.image_generation=false", "-c", `web_search="disabled"`, "-c", "features.apps=false", "-c", "features.skill_mcp_dependency_install=false", "exec"}
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
	if request.ImageGeneration {
		if request.Engine != "codex" || request.NativeID != "" {
			return nil, fmt.Errorf("image generation requires an isolated Codex turn")
		}
		args = CodexImageArgs(request.Images)
	}
	if request.Engine == "claude" {
		args = []string{"--restricted", "-p", "--output-format", "stream-json", "--verbose", "--tools", "", "--disallowedTools", "mcp__*", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--setting-sources", ""}
		if request.NativeID != "" {
			args = append(args, "--resume", request.NativeID)
		}
	}
	if !request.ImageGeneration && ValidModel(request.Model) && ValidEffort(request.Engine, request.Effort) {
		if request.Engine == "claude" {
			if request.Model != "" {
				args = append(args, "--model", request.Model)
			}
			if request.Effort != "" {
				args = append(args, "--effort", request.Effort)
			}
		} else {
			overrides := []string{}
			if request.Model != "" {
				overrides = append(overrides, "-c", `model="`+request.Model+`"`)
			}
			if request.Effort != "" {
				overrides = append(overrides, "-c", `model_reasoning_effort="`+request.Effort+`"`)
			}
			args = append(overrides, args...) // global options go before "exec"
		}
	}
	stdin := request.Prompt
	if !request.ImageGeneration && len(request.Images) > 0 {
		// Attached media for the planner to look at. It never gains tools or file access.
		if request.Engine == "claude" {
			if stdin, err = claudeImageInput(request.Prompt, request.Images); err != nil {
				return nil, err
			}
			args = append(args, "--input-format", "stream-json")
		} else {
			last := len(args) - 1 // keep the trailing "-" (prompt from stdin)
			withImages := append([]string{}, args[:last]...)
			for _, path := range request.Images {
				withImages = append(withImages, "--image="+path)
			}
			args = append(withImages, "-")
		}
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = request.Workspace
	cmd.Env = SafeEnvironment(os.Environ())
	if request.ImageGeneration {
		filtered := []string{}
		for _, entry := range cmd.Env {
			key, _, _ := strings.Cut(entry, "=")
			if key != "OPENAI_API_KEY" && key != "ANTHROPIC_API_KEY" {
				filtered = append(filtered, entry)
			}
		}
		cmd.Env = filtered
	}
	cmd.Stdin = strings.NewReader(stdin)
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
			return nil, fmt.Errorf("%s couldn't answer. It may be signed out or out of usage: check it in Settings → AI connections, or switch the conversation to another AI, then try again", engineLabel(request.Engine))
		case "result":
			if event.IsError || event.Subtype != "success" {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				return nil, fmt.Errorf("Claude Code couldn't answer. It may be signed out or out of usage: check it in Settings → AI connections, or switch the conversation to another AI, then try again")
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

func engineLabel(engine string) string {
	if engine == "claude" {
		return "Claude Code"
	}
	return "Codex"
}

// claudeImageInput is one stream-json user message carrying the images and the prompt.
func claudeImageInput(prompt string, images []string) (string, error) {
	content := []map[string]any{}
	for _, path := range images {
		data, err := os.ReadFile(path)
		if err != nil || len(data) > 4<<20 {
			continue // Unreadable or oversized previews are left out rather than failing the turn.
		}
		media := "image/jpeg"
		switch strings.ToLower(filepath.Ext(path)) {
		case ".png":
			media = "image/png"
		case ".gif":
			media = "image/gif"
		case ".webp":
			media = "image/webp"
		}
		content = append(content, map[string]any{"type": "image", "source": map[string]string{"type": "base64", "media_type": media, "data": base64.StdEncoding.EncodeToString(data)}})
	}
	content = append(content, map[string]any{"type": "text", "text": prompt})
	line, err := json.Marshal(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": content}})
	return string(line) + "\n", err
}

// ValidModel accepts a model alias or full name; empty means the default.
func ValidModel(model string) bool {
	if len(model) > 80 {
		return false
	}
	for _, r := range model {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-:[]", r)) {
			return false
		}
	}
	return true
}

// Efforts lists the reasoning levels each tool accepts, lowest first.
func Efforts(engine string) []string {
	if engine == "claude" {
		return []string{"low", "medium", "high", "xhigh", "max"}
	}
	return []string{"low", "medium", "high", "xhigh"}
}

func ValidEffort(engine, effort string) bool {
	if effort == "" {
		return true
	}
	for _, e := range Efforts(engine) {
		if e == effort {
			return true
		}
	}
	return false
}
