package models

import "encoding/json"

// ReferenceApproved keeps explicit user imports and unmarked legacy references
// usable, while generated candidates need an explicit current human review.
func ReferenceApproved(metadata json.RawMessage) bool {
	var m struct {
		Review string `json:"consistency_review"`
	}
	if json.Unmarshal(metadata, &m) != nil {
		return len(metadata) == 0
	}
	return m.Review == "" || m.Review == "approved"
}
