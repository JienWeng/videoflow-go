package director

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// AuthState is what the UI needs to show for one engine's sign-in.
type AuthState struct {
	Engine    string `json:"engine"`
	Installed bool   `json:"installed"`
	SignedIn  bool   `json:"signed_in"`
	// SignedOut is true only when the CLI clearly answered "not signed in".
	SignedOut bool   `json:"signed_out,omitempty"`
	Method    string `json:"method,omitempty"`
	Login     *Login `json:"login,omitempty"`
}

// Login is an in-progress browser sign-in started from the app.
type Login struct {
	Status    string    `json:"status"` // starting, waiting, verifying, complete, failed, cancelled
	URL       string    `json:"url,omitempty"`
	Code      string    `json:"code,omitempty"`       // Codex device code the user types on the website
	NeedsCode bool      `json:"needs_code,omitempty"` // Claude shows a code on the website that must be pasted back
	Message   string    `json:"message,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`

	cancel context.CancelFunc
	stdin  io.WriteCloser
	output *lockedBuffer
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.buf.Len() < 64<<10 {
		b.buf.Write(p)
	}
	return len(p), nil
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

var (
	ansiPattern       = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	urlPattern        = regexp.MustCompile(`https://[^\s'"<>]+`)
	deviceCodePattern = regexp.MustCompile(`\b[A-Z0-9]{4}-[A-Z0-9]{4,5}\b`)
)

const loginTTL = 15 * time.Minute

// AuthManager runs at most one sign-in per engine.
type AuthManager struct {
	runner *Runner
	mu     sync.Mutex
	logins map[string]*Login
	cache  map[string]cachedAuth
	// epoch counts the sign-ins that have ended for an engine. A check that began before
	// one ended may carry an out-of-date answer, so it must not be remembered.
	epoch map[string]int
}

type cachedAuth struct {
	signedIn  bool
	signedOut bool
	method    string
	at        time.Time
}

const authCacheTTL = 30 * time.Second

func NewAuthManager(r *Runner) *AuthManager {
	return &AuthManager{runner: r, logins: map[string]*Login{}, cache: map[string]cachedAuth{}, epoch: map[string]int{}}
}

// Status checks the real sign-in state, not just whether the CLI is installed.
func (m *AuthManager) Status(ctx context.Context, engine string) AuthState {
	state := AuthState{Engine: engine}
	path, err := m.runner.Available(engine)
	if err != nil {
		return state
	}
	state.Installed = true
	m.mu.Lock()
	cached, ok := m.cache[engine]
	m.mu.Unlock()
	if ok && time.Since(cached.at) < authCacheTTL {
		state.SignedIn, state.SignedOut, state.Method = cached.signedIn, cached.signedOut, cached.method
	} else {
		var definitive bool
		m.mu.Lock()
		began := m.epoch[engine]
		m.mu.Unlock()
		state.SignedIn, state.SignedOut, state.Method, definitive = m.check(ctx, engine, path)
		m.mu.Lock()
		changed := m.epoch[engine] != began
		began = m.epoch[engine]
		m.mu.Unlock()
		if changed {
			// A sign-in ended while this check was running, so ask again for the new answer.
			state.SignedIn, state.SignedOut, state.Method, definitive = m.check(ctx, engine, path)
		}
		// Only remember answers the CLI actually gave. A check cut short (the browser
		// navigated away, or the CLI was slow) must not read as "signed out" for 30s.
		if definitive {
			m.mu.Lock()
			if m.epoch[engine] == began {
				m.cache[engine] = cachedAuth{state.SignedIn, state.SignedOut, state.Method, time.Now()}
			}
			m.mu.Unlock()
		} else if ok {
			state.SignedIn, state.SignedOut, state.Method = cached.signedIn, cached.signedOut, cached.method
		}
	}
	m.mu.Lock()
	if l := m.logins[engine]; l != nil {
		copy := *l
		state.Login = &copy
	}
	m.mu.Unlock()
	return state
}

func (m *AuthManager) check(ctx context.Context, engine, path string) (signedIn, signedOut bool, method string, definitive bool) {
	// Detached from the HTTP request so a page change can't kill the check midway.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	defer func() { definitive = ctx.Err() == nil }()
	if engine == "claude" {
		out, _ := exec.CommandContext(ctx, path, "auth", "status").Output()
		var s struct {
			LoggedIn   bool   `json:"loggedIn"`
			AuthMethod string `json:"authMethod"`
		}
		if json.Unmarshal(out, &s) == nil {
			signedIn, signedOut = s.LoggedIn, !s.LoggedIn
			if s.LoggedIn {
				method = friendlyMethod(s.AuthMethod)
			}
		}
	} else {
		out, err := exec.CommandContext(ctx, path, "login", "status").CombinedOutput()
		text := strings.TrimSpace(ansiPattern.ReplaceAllString(string(out), ""))
		signedOut = strings.Contains(strings.ToLower(text), "not logged in")
		if err == nil && strings.Contains(strings.ToLower(text), "logged in") && !signedOut {
			signedIn = true
			if i := strings.Index(text, "using "); i >= 0 {
				method = strings.TrimSpace(text[i+len("using "):])
			}
		}
	}
	return signedIn, signedOut, method, true
}

func friendlyMethod(method string) string {
	switch method {
	case "claude.ai", "oauth_token", "claudeai":
		return "Claude subscription"
	case "api_key", "console":
		return "Anthropic Console"
	}
	return method
}

// Start begins a browser sign-in and waits briefly for the link to appear.
func (m *AuthManager) Start(engine string) (*Login, error) {
	path, err := m.runner.Available(engine)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if old := m.logins[engine]; old != nil && (old.Status == "starting" || old.Status == "waiting" || old.Status == "verifying") {
		old.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), loginTTL)
	args := []string{"login", "--device-auth"}
	if engine == "claude" {
		args = []string{"auth", "login", "--claudeai"}
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = append(cmd.Environ(), "BROWSER=true", "NO_COLOR=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		m.mu.Unlock()
		cancel()
		return nil, err
	}
	out := &lockedBuffer{}
	cmd.Stdout, cmd.Stderr = out, out
	login := &Login{Status: "starting", ExpiresAt: time.Now().Add(loginTTL), cancel: cancel, stdin: stdin, output: out}
	if err = cmd.Start(); err != nil {
		m.mu.Unlock()
		cancel()
		return nil, fmt.Errorf("could not start %s sign-in: %w", engine, err)
	}
	m.logins[engine] = login
	m.mu.Unlock()

	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.cache, engine)
		m.epoch[engine]++
		switch {
		case login.Status == "cancelled":
		case err == nil:
			login.Status, login.Message = "complete", "Signed in."
		case ctx.Err() == context.DeadlineExceeded:
			login.Status, login.Message = "failed", "The sign-in link expired. Start again."
		default:
			login.Status, login.Message = "failed", loginFailure(out.String())
		}
		login.URL, login.Code = "", ""
		cancel()
	}()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if m.parse(engine, login) {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if login.Status == "starting" {
		login.cancel()
		login.Status, login.Message = "failed", "The sign-in link did not appear. Check that the backend can reach the internet, then try again."
	}
	copy := *login
	return &copy, nil
}

// parse pulls the sign-in link (and Codex device code) out of the CLI output.
func (m *AuthManager) parse(engine string, login *Login) bool {
	text := ansiPattern.ReplaceAllString(login.output.String(), "")
	m.mu.Lock()
	defer m.mu.Unlock()
	if login.Status != "starting" {
		return true
	}
	urls := urlPattern.FindAllString(text, -1)
	if engine == "claude" {
		for _, u := range urls {
			if strings.Contains(u, "oauth") || strings.Contains(u, "claude") {
				login.URL, login.NeedsCode, login.Status = u, true, "waiting"
				return true
			}
		}
		return false
	}
	code := deviceCodePattern.FindString(text)
	if len(urls) > 0 && code != "" {
		login.URL, login.Code, login.Status = urls[0], code, "waiting"
		return true
	}
	return false
}

// SubmitCode sends the code Claude's website shows after sign-in.
func (m *AuthManager) SubmitCode(engine, code string) error {
	code = strings.TrimSpace(code)
	if code == "" || len(code) > 512 || strings.ContainsAny(code, "\r\n") {
		return fmt.Errorf("paste the code exactly as shown on the sign-in page")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	login := m.logins[engine]
	if login == nil || !login.NeedsCode || login.Status != "waiting" {
		return fmt.Errorf("start sign-in first")
	}
	if _, err := io.WriteString(login.stdin, code+"\n"); err != nil {
		return fmt.Errorf("sign-in ended; start again")
	}
	login.Status, login.Message = "verifying", "Checking the code…"
	return nil
}

func (m *AuthManager) Cancel(engine string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if login := m.logins[engine]; login != nil && login.Status != "complete" && login.Status != "failed" {
		login.Status, login.Message, login.URL, login.Code = "cancelled", "Sign-in cancelled.", "", ""
		login.cancel()
	}
}

// loginFailure gives a short reason without echoing links, tokens or codes.
func loginFailure(output string) string {
	text := strings.ToLower(ansiPattern.ReplaceAllString(output, ""))
	switch {
	case strings.Contains(text, "invalid") && strings.Contains(text, "code"), strings.Contains(text, "invalid_grant"):
		return "That code was not accepted. Start again and paste the newest code."
	case strings.Contains(text, "expired"):
		return "The code expired. Start again."
	case strings.Contains(text, "network") || strings.Contains(text, "enotfound") || strings.Contains(text, "timed out"):
		return "The backend could not reach the sign-in service. Check its internet connection."
	}
	return "Sign-in did not finish. Start again; if it keeps failing, use the terminal command under Technical details."
}
