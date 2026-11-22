package models

import "encoding/json"

// The states a render job or operation moves through. The values are stored in the
// database and sent to the frontend, so they must not change.
const (
	StatusPending                = "pending"
	StatusRunning                = "running"
	StatusSucceeded              = "succeeded"
	StatusFailed                 = "failed"
	StatusReconciliationRequired = "reconciliation_required"
)

// Active reports whether the job still holds its scene: waiting, being made, or waiting
// on a video service that has not said how an earlier request ended.
func (j *RenderJob) Active() bool {
	return j.Status == StatusPending || j.Status == StatusRunning || j.Status == StatusReconciliationRequired
}

// IDList reads a saved list of ids (a scene's cast, its attached assets, a character's
// pictures). A missing or unreadable value is an empty list.
func IDList(raw json.RawMessage) []string {
	ids := []string{}
	_ = json.Unmarshal(raw, &ids)
	if ids == nil {
		return []string{}
	}
	return ids
}
