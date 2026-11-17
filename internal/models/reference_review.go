package models

import "encoding/json"

// ReferenceApproved reports whether a reference may be used. Manual approval is
// optional; legacy pending references are usable, while explicit rejection is retained.
func ReferenceApproved(metadata json.RawMessage) bool {
	var m struct {
		Review string `json:"consistency_review"`
	}
	if json.Unmarshal(metadata, &m) != nil {
		return len(metadata) == 0
	}
	return m.Review == "" || m.Review == "approved" || m.Review == "pending" || m.Review == "ready"
}
