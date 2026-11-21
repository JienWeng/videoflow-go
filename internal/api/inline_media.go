package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"videoflow-go/internal/models"
)

// withoutInlineMedia swaps every inline data: URL for a short marker. Stored
// requests keep the bytes; responses and prompts only need the small fields.
func withoutInlineMedia(raw json.RawMessage) json.RawMessage {
	if !bytes.Contains(raw, []byte(`"data:`)) {
		return raw
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return raw
	}
	out, err := json.Marshal(stripInlineMedia(value))
	if err != nil {
		return raw
	}
	return out
}

func stripInlineMedia(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			v[key] = stripInlineMedia(item)
		}
	case []any:
		for i, item := range v {
			v[i] = stripInlineMedia(item)
		}
	case string:
		if strings.HasPrefix(v, "data:") {
			kind, _, _ := strings.Cut(strings.TrimPrefix(v, "data:"), ";")
			return fmt.Sprintf("[inline %s, %d bytes, not shown]", kind, len(v))
		}
	}
	return value
}

func renderJobView(job models.RenderJob) models.RenderJob {
	job.RequestJSON = withoutInlineMedia(job.RequestJSON)
	job.ResponseJSON = withoutInlineMedia(job.ResponseJSON)
	return job
}
