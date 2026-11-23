package providers

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// Google's errors follow google.rpc.Status: {"error": {"code", "status", "message",
// "details": [...]}}. The details carry the reason (ErrorInfo), the field at fault
// (BadRequest.fieldViolations) and similar. Everything kept from them is redacted: the
// key is removed, long base64 runs (picture bytes) are replaced and each piece is short,
// so no prompt or picture reaches a saved job or a log in full.

// geminiErrorBodyLimit keeps enough of an error body to read its details.
const geminiErrorBodyLimit = 16 << 10

// geminiPieceLimit caps each message or detail kept from Google.
const geminiPieceLimit = 300

type geminiStatus struct {
	Code    json.RawMessage  `json:"code"`
	Status  string           `json:"status"`
	Message string           `json:"message"`
	Details []map[string]any `json:"details"`
}

var geminiBase64Run = regexp.MustCompile(`[A-Za-z0-9+/=_-]{200,}`)

// geminiScrub makes text from Google safe to keep: no key, no picture data, no long
// echo of the prompt.
func geminiScrub(text, key, prompt string) string {
	text = strings.TrimSpace(text)
	if key != "" {
		text = strings.ReplaceAll(text, key, "[redacted]")
	}
	if prompt = strings.TrimSpace(prompt); len(prompt) >= 40 {
		text = strings.ReplaceAll(text, prompt, "[prompt]")
		// An echo is often cut short, so its opening words are enough to recognise it.
		if at := strings.Index(text, prompt[:40]); at >= 0 {
			text = strings.TrimSpace(text[:at]) + " [prompt]"
		}
	}
	text = geminiBase64Run.ReplaceAllString(text, "[data]")
	if len(text) > geminiPieceLimit {
		cut := geminiPieceLimit
		for cut > 0 && !utf8Start(text[cut]) {
			cut--
		}
		text = text[:cut] + "…"
	}
	return strings.TrimSpace(text)
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

// code is Google's error code as text: a number (400, or 13 in an operation) or a name
// such as "invalid_request".
func (s geminiStatus) code() string {
	raw := strings.TrimSpace(string(s.Code))
	if raw == "" || raw == "null" {
		return ""
	}
	var name string
	if json.Unmarshal(s.Code, &name) == nil {
		return name
	}
	var number json.Number
	if json.Unmarshal(s.Code, &number) == nil {
		return number.String()
	}
	return ""
}

// describe is the status and message, then the code when it adds anything to the HTTP
// status, then the details: "INVALID_ARGUMENT Request contains an invalid argument.
// (field input[1].data: Image too large; reason IMAGE_TOO_LARGE)".
func (s geminiStatus) describe(httpStatus int, key, prompt string) string {
	head := strings.TrimSpace(geminiScrub(s.Status, key, "") + " " + geminiScrub(s.Message, key, prompt))
	extra := []string{}
	if code := s.code(); code != "" && code != strconv.Itoa(httpStatus) && !strings.EqualFold(code, s.Status) {
		extra = append(extra, "code "+geminiScrub(code, key, ""))
	}
	for _, detail := range s.Details {
		extra = append(extra, geminiDetail(detail, key, prompt, s.Message)...)
	}
	if len(extra) > 8 {
		extra = append(extra[:8], "…")
	}
	if len(extra) > 0 {
		head = strings.TrimSpace(head + " (" + strings.Join(extra, "; ") + ")")
	}
	return head
}

// geminiDetail reads the parts of one google.rpc detail that say what went wrong.
func geminiDetail(detail map[string]any, key, prompt, message string) []string {
	out := []string{}
	text := func(v any) string {
		s, _ := v.(string)
		return geminiScrub(s, key, prompt)
	}
	if reason := text(detail["reason"]); reason != "" {
		out = append(out, "reason "+reason)
	}
	for _, list := range []string{"fieldViolations", "field_violations", "violations"} {
		items, _ := detail[list].([]any)
		for _, item := range items {
			v, _ := item.(map[string]any)
			subject := text(v["field"])
			if subject == "" {
				subject = text(v["subject"])
			}
			description := text(v["description"])
			if subject == "" && description == "" {
				continue
			}
			out = append(out, strings.TrimSpace("field "+subject+": "+description))
		}
	}
	if delay := text(detail["retryDelay"]); delay != "" {
		out = append(out, "retry after "+delay)
	}
	if localized := text(detail["message"]); localized != "" && localized != geminiScrub(message, key, prompt) {
		out = append(out, localized)
	}
	if len(out) == 0 {
		if kind := text(detail["@type"]); kind != "" {
			out = append(out, kind[strings.LastIndex(kind, ".")+1:])
		}
	}
	return out
}

// geminiHTTPError replaces Google's JSON error body with its status, message, code and
// details, scrubbed. A body that is not Google's JSON is kept as it was (already
// redacted and shortened by mediaHTTP).
func geminiHTTPError(err error) error {
	return geminiHTTPErrorFor(err, "")
}

// geminiHTTPErrorFor is geminiHTTPError for a request whose prompt must not be echoed.
func geminiHTTPErrorFor(err error, prompt string) error {
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		return err
	}
	var body struct {
		Error geminiStatus `json:"error"`
	}
	if json.Unmarshal([]byte(httpErr.Message), &body) == nil && (body.Error.Message != "" || body.Error.Status != "") {
		httpErr.Message = body.Error.describe(httpErr.Status, "", prompt)
	} else if len(httpErr.Message) > 1000 {
		httpErr.Message = geminiScrub(httpErr.Message, "", prompt)
	}
	return err
}

// geminiStatusText is the same summary for an error inside a finished interaction.
func geminiStatusText(raw json.RawMessage, key string) string {
	var s geminiStatus
	if json.Unmarshal(raw, &s) != nil {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			return geminiScrub(text, key, "")
		}
		return ""
	}
	return s.describe(0, key, "")
}
