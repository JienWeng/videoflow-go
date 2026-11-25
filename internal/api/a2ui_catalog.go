package api

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

// The chat assistant may answer with a small A2UI (https://a2ui.org, v1.0) interface of
// its own in its `ui` field: a question with choices, a comparison, a picture. Only a
// few components of the A2UI basic catalog are allowed, with literal values only, and
// none of them can spend money: every button and choice the assistant writes is
// rewritten to the videoflow.reply action, which comes back as the user's next chat
// message. Paid steps stay the app's own cards (the VideoFlowAction component).

const (
	a2uiMaxComponents = 30
	a2uiMaxChildren   = 12
	a2uiMaxDepth      = 8
	a2uiMaxText       = 500
	a2uiMaxLabel      = 80
	a2uiMaxReply      = 300
	a2uiMaxBytes      = 8 << 10
	// a2uiReplyAction is the only action the assistant's components may send.
	a2uiReplyAction = "videoflow.reply"
	// a2uiIDPrefix keeps the assistant's component ids apart from the app's.
	a2uiIDPrefix = "ui-"
	// a2uiUIRoot is the id the assistant's interface hangs from, after the prefix.
	a2uiUIRoot = a2uiIDPrefix + "root"
)

var a2uiComponentID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

var a2uiTextVariants = map[string]bool{"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "body": true, "caption": true}

// sanitizeA2UI checks the assistant's `ui` against the allowlist and returns it as the
// A2UI components the app sends, ids prefixed and rooted at ui-root. Anything that does
// not pass is dropped whole (nil); the text reply is still shown.
func sanitizeA2UI(raw json.RawMessage) []map[string]any {
	if len(raw) == 0 || string(raw) == "null" || len(raw) > 4*a2uiMaxBytes {
		return nil
	}
	var in []map[string]any
	if json.Unmarshal(raw, &in) != nil || len(in) == 0 || len(in) > a2uiMaxComponents {
		return nil
	}
	byID := map[string]map[string]any{}
	for _, c := range in {
		id, _ := c["id"].(string)
		if !a2uiComponentID.MatchString(id) || byID[id] != nil {
			return nil
		}
		byID[id] = c
	}
	if byID["root"] == nil {
		return nil
	}
	out := []map[string]any{}
	seen := map[string]bool{}
	var walk func(id string, depth int) bool
	walk = func(id string, depth int) bool {
		c := byID[id]
		if c == nil || seen[id] || depth > a2uiMaxDepth {
			return false // missing, shared or cyclic, or too deep
		}
		seen[id] = true
		clean, children, ok := sanitizeA2UIComponent(c)
		if !ok {
			return false
		}
		clean["id"] = a2uiIDPrefix + id
		out = append(out, clean)
		for _, child := range children {
			if !walk(child, depth+1) {
				return false
			}
		}
		return true
	}
	if !walk("root", 0) || len(seen) != len(byID) {
		return nil
	}
	if encoded, _ := json.Marshal(out); len(encoded) > a2uiMaxBytes {
		return nil
	}
	return out
}

// sanitizeA2UIComponent rebuilds one component from its allowed properties and names
// the children it refers to (unprefixed).
func sanitizeA2UIComponent(c map[string]any) (map[string]any, []string, bool) {
	kind, _ := c["component"].(string)
	out := map[string]any{"component": kind}
	switch kind {
	case "Text":
		text, ok := a2uiString(c["text"], a2uiMaxText)
		if !ok {
			return nil, nil, false
		}
		out["text"] = text
		if v, has := c["variant"]; has {
			s, _ := v.(string)
			if !a2uiTextVariants[s] {
				return nil, nil, false
			}
			out["variant"] = s
		}
		return out, nil, true
	case "Image":
		url, ok := a2uiString(c["url"], a2uiMaxText)
		if !ok || !(strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "/") && !strings.HasPrefix(url, "//")) {
			return nil, nil, false
		}
		out["url"] = url
		if d, has := c["description"]; has {
			s, ok := a2uiString(d, a2uiMaxLabel*2)
			if !ok {
				return nil, nil, false
			}
			out["description"] = s
		}
		return out, nil, true
	case "Row", "Column", "List":
		list, ok := c["children"].([]any)
		if !ok || len(list) == 0 || len(list) > a2uiMaxChildren {
			return nil, nil, false
		}
		children := []string{}
		prefixed := []string{}
		for _, item := range list {
			id, _ := item.(string)
			if id == "" {
				return nil, nil, false
			}
			children = append(children, id)
			prefixed = append(prefixed, a2uiIDPrefix+id)
		}
		out["children"] = prefixed
		return out, children, true
	case "Card":
		child, _ := c["child"].(string)
		if child == "" {
			return nil, nil, false
		}
		out["child"] = a2uiIDPrefix + child
		return out, []string{child}, true
	case "Button":
		child, _ := c["child"].(string)
		action, _ := c["action"].(map[string]any)
		event, _ := action["event"].(map[string]any)
		context, _ := event["context"].(map[string]any)
		reply, ok := a2uiString(context["reply"], a2uiMaxReply)
		if child == "" || !ok {
			return nil, nil, false
		}
		out["child"] = a2uiIDPrefix + child
		out["action"] = map[string]any{"event": map[string]any{"name": a2uiReplyAction, "context": map[string]any{"reply": reply}}}
		if v, has := c["variant"]; has {
			if s, _ := v.(string); s == "primary" || s == "borderless" {
				out["variant"] = s
			} else {
				return nil, nil, false
			}
		}
		return out, []string{child}, true
	case "ChoicePicker":
		list, ok := c["options"].([]any)
		if !ok || len(list) < 2 || len(list) > 8 {
			return nil, nil, false
		}
		options := []map[string]any{}
		for _, item := range list {
			o, _ := item.(map[string]any)
			label, ok1 := a2uiString(o["label"], a2uiMaxLabel)
			value, ok2 := a2uiString(o["value"], a2uiMaxReply)
			if !ok1 || !ok2 {
				return nil, nil, false
			}
			options = append(options, map[string]any{"label": label, "value": value})
		}
		out["options"] = options
		out["variant"] = "mutuallyExclusive"
		// A pick is sent at once as the user's reply; the binding only marks the choice.
		out["value"] = map[string]any{"@path": "/choice"}
		return out, nil, true
	case "Divider":
		axis := "horizontal"
		if v, has := c["axis"]; has {
			if s, _ := v.(string); s == "horizontal" || s == "vertical" {
				axis = s
			} else {
				return nil, nil, false
			}
		}
		out["axis"] = axis
		return out, nil, true
	}
	return nil, nil, false
}

// a2uiString is a literal, non-empty string of at most limit characters. Bindings
// ({"@path": …}) and function calls are refused: the assistant writes values only.
func a2uiString(v any, limit int) (string, bool) {
	s, ok := v.(string)
	s = strings.TrimSpace(s)
	return s, ok && s != "" && utf8.RuneCountInString(s) <= limit
}
