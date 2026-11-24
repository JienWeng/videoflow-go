package api

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// patchOp is one RFC 6902 JSON Patch operation, as AG-UI's STATE_DELTA carries them.
type patchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

// MarshalJSON keeps "value" on add and replace even when it is null or empty, and
// leaves it off remove.
func (p patchOp) MarshalJSON() ([]byte, error) {
	if p.Op == "remove" {
		return json.Marshal(struct {
			Op   string `json:"op"`
			Path string `json:"path"`
		}{p.Op, p.Path})
	}
	return json.Marshal(struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value any    `json:"value"`
	}{p.Op, p.Path, p.Value})
}

// jsonValue is v as plain JSON values (maps, slices, strings, float64, bool, nil), the
// form diffJSON compares.
func jsonValue(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out any
	_ = json.Unmarshal(raw, &out) // just marshalled, so it decodes
	return out
}

// diffJSON is the JSON Patch that turns from into to. Objects are compared key by key
// and arrays of the same length item by item; an array whose length changed is
// replaced whole, which keeps every patch simple to apply.
func diffJSON(from, to any) []patchOp {
	ops := []patchOp{}
	diffAt("", from, to, &ops)
	return ops
}

func diffAt(path string, from, to any, ops *[]patchOp) {
	if reflect.DeepEqual(from, to) {
		return
	}
	switch a := from.(type) {
	case map[string]any:
		b, ok := to.(map[string]any)
		if !ok {
			break
		}
		for _, key := range sortedKeys(a) {
			if _, kept := b[key]; !kept {
				*ops = append(*ops, patchOp{Op: "remove", Path: path + "/" + pointerToken(key)})
			}
		}
		for _, key := range sortedKeys(b) {
			if old, had := a[key]; had {
				diffAt(path+"/"+pointerToken(key), old, b[key], ops)
			} else {
				*ops = append(*ops, patchOp{Op: "add", Path: path + "/" + pointerToken(key), Value: b[key]})
			}
		}
		return
	case []any:
		b, ok := to.([]any)
		if !ok || len(a) != len(b) {
			break
		}
		for i := range a {
			diffAt(path+"/"+strconv.Itoa(i), a[i], b[i], ops)
		}
		return
	}
	*ops = append(*ops, patchOp{Op: "replace", Path: path, Value: to})
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// pointerToken escapes a key for a JSON Pointer (RFC 6901).
func pointerToken(key string) string {
	return strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}

// applyJSONPatch applies add, remove and replace operations to doc, a plain JSON
// value, and returns the result. It is what a client does with a STATE_DELTA; the
// tests use it to check every delta applies.
func applyJSONPatch(doc any, ops []patchOp) (any, error) {
	for _, op := range ops {
		var err error
		if doc, err = applyPatchOp(doc, op); err != nil {
			return nil, err
		}
	}
	return doc, nil
}

func applyPatchOp(doc any, op patchOp) (any, error) {
	if op.Path == "" {
		if op.Op == "remove" {
			return nil, nil
		}
		return op.Value, nil
	}
	tokens := strings.Split(op.Path[1:], "/")
	for i, t := range tokens {
		tokens[i] = strings.ReplaceAll(strings.ReplaceAll(t, "~1", "/"), "~0", "~")
	}
	return patchAt(doc, tokens, op)
}

func patchAt(node any, tokens []string, op patchOp) (any, error) {
	key, last := tokens[0], len(tokens) == 1
	switch n := node.(type) {
	case map[string]any:
		if !last {
			child, ok := n[key]
			if !ok {
				return nil, fmt.Errorf("path %s: no %q", op.Path, key)
			}
			updated, err := patchAt(child, tokens[1:], op)
			n[key] = updated
			return n, err
		}
		_, exists := n[key]
		switch {
		case op.Op == "add":
			n[key] = op.Value
		case !exists:
			return nil, fmt.Errorf("path %s: no %q to %s", op.Path, key, op.Op)
		case op.Op == "replace":
			n[key] = op.Value
		case op.Op == "remove":
			delete(n, key)
		default:
			return nil, fmt.Errorf("unsupported op %s", op.Op)
		}
		return n, nil
	case []any:
		i, err := strconv.Atoi(key)
		if key == "-" && last && op.Op == "add" {
			return append(n, op.Value), nil
		}
		if err != nil || i < 0 || i > len(n) || (i == len(n) && !(last && op.Op == "add")) {
			return nil, fmt.Errorf("path %s: bad index %q", op.Path, key)
		}
		if !last {
			updated, err := patchAt(n[i], tokens[1:], op)
			n[i] = updated
			return n, err
		}
		switch op.Op {
		case "add":
			return append(n[:i], append([]any{op.Value}, n[i:]...)...), nil
		case "replace":
			n[i] = op.Value
			return n, nil
		case "remove":
			return append(n[:i], n[i+1:]...), nil
		}
		return nil, fmt.Errorf("unsupported op %s", op.Op)
	}
	return nil, fmt.Errorf("path %s: %q is not inside an object or array", op.Path, key)
}
