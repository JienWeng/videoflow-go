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
	args, stdin, err := turnArgs(request)
	if err != nil {
		return nil, err
	}
	cmd := turnCommand(ctx, executable, args, request, stdin)
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
	turn := &turnReader{engine: request.Engine, native: request.NativeID, saveNative: saveNative}
	for scanner.Scan() {
		if err = turn.read(scanner.Bytes()); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil, err
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
	return turn.answer()
}

// turnArgs builds the CLI arguments for one turn and the text sent on its stdin.
func turnArgs(request Request) ([]string, string, error) {
	args := CodexArgs(request.NativeID)
	if request.ImageGeneration {
		if request.Engine != "codex" || request.NativeID != "" {
			return nil, "", fmt.Errorf("image generation requires an isolated Codex turn")
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
		args = withModelArgs(request, args)
	}
	if !request.ImageGeneration && len(request.Images) > 0 {
		return withAttachedImages(request, args)
	}
	return args, request.Prompt, nil
}

// withModelArgs adds the chosen model and reasoning effort.
func withModelArgs(request Request, args []string) []string {
	if request.Engine == "claude" {
		if request.Model != "" {
			args = append(args, "--model", request.Model)
		}
		if request.Effort != "" {
			args = append(args, "--effort", request.Effort)
		}
		return args
	}
	overrides := []string{}
	if request.Model != "" {
		overrides = append(overrides, "-c", `model="`+request.Model+`"`)
	}
	if request.Effort != "" {
		overrides = append(overrides, "-c", `model_reasoning_effort="`+request.Effort+`"`)
	}
	return append(overrides, args...) // global options go before "exec"
}

// withAttachedImages attaches media for the planner to look at. It never gains tools or
// file access.
func withAttachedImages(request Request, args []string) ([]string, string, error) {
	if request.Engine == "claude" {
		stdin, err := claudeImageInput(request.Prompt, request.Images)
		if err != nil {
			return nil, "", err
		}
		return append(args, "--input-format", "stream-json"), stdin, nil
	}
	last := len(args) - 1 // keep the trailing "-" (prompt from stdin)
	withImages := append([]string{}, args[:last]...)
	for _, path := range request.Images {
		withImages = append(withImages, "--image="+path)
	}
	return append(withImages, "-"), request.Prompt, nil
}

// turnCommand prepares the CLI process without the service API keys in its environment.
func turnCommand(ctx context.Context, executable string, args []string, request Request, stdin string) *exec.Cmd {
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
	return cmd
}

// turnEvent is one line of the CLI's JSON protocol output (Codex or Claude Code).
type turnEvent struct {
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

// turnReader follows a turn's protocol events: the native session it belongs to, the
// answer so far and whether the turn completed.
type turnReader struct {
	engine     string
	native     string
	result     string
	completed  bool
	saveNative func(string) error
}

// read handles one protocol event. An error ends the turn.
func (t *turnReader) read(line []byte) error {
	var event turnEvent
	if json.Unmarshal(line, &event) != nil {
		return fmt.Errorf("%s returned an invalid protocol event", t.engine)
	}
	id := event.ThreadID
	if t.engine == "claude" {
		id = event.SessionID
	}
	if id != "" {
		if len(id) > 160 || (t.native != "" && t.native != id) {
			return fmt.Errorf("native session identity mismatch")
		}
		if t.native == "" {
			t.native = id
		}
		if err := t.saveNative(id); err != nil {
			return err
		}
	}
	switch event.Type {
	case "item.completed":
		if event.Item.Type == "agent_message" {
			t.result = event.Item.Text
		}
	case "turn.completed":
		t.completed = true
	case "turn.failed", "error":
		return fmt.Errorf("%s couldn't answer. It may be signed out or out of usage: check it in Settings → AI connections, or switch the conversation to another AI, then try again", engineLabel(t.engine))
	case "result":
		if event.IsError || event.Subtype != "success" {
			return fmt.Errorf("Claude Code couldn't answer. It may be signed out or out of usage: check it in Settings → AI connections, or switch the conversation to another AI, then try again")
		}
		t.result = event.Result
		t.completed = true
	}
	return nil
}

// answer is the structured JSON a finished turn replied with.
func (t *turnReader) answer() (json.RawMessage, error) {
	if !t.completed || t.native == "" || strings.TrimSpace(t.result) == "" {
		return nil, fmt.Errorf("%s turn was interrupted or returned no result; it will not be replayed automatically", t.engine)
	}
	structured, ok := structuredJSON(t.result)
	if !ok {
		return nil, fmt.Errorf("%s returned invalid structured JSON; review the session before retrying", t.engine)
	}
	return structured, nil
}

// structuredJSON finds the JSON answer in an agent's reply. Agents are told to answer
// with JSON alone, but now and then one wraps it in a code fence or adds a sentence
// before or after it; the first complete object or array in the reply is the answer.
func structuredJSON(reply string) (json.RawMessage, bool) {
	reply = strings.TrimSpace(reply)
	if json.Valid([]byte(reply)) {
		return json.RawMessage(reply), reply != ""
	}
	for start := strings.IndexAny(reply, "{["); start >= 0; {
		var value json.RawMessage
		if json.NewDecoder(strings.NewReader(reply[start:])).Decode(&value) == nil {
			return value, true
		}
		next := strings.IndexAny(reply[start+1:], "{[")
		if next < 0 {
			break
		}
		start += next + 1
	}
	return nil, false
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
