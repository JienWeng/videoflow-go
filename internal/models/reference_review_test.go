package models

import "testing"

func TestReferencesDoNotRequireManualApproval(t *testing.T) {
	for _, raw := range []string{`{}`, `{"consistency_review":"pending"}`, `{"consistency_review":"ready"}`, `{"consistency_review":"approved"}`} {
		if !ReferenceApproved([]byte(raw)) {
			t.Errorf("reference should be usable without manual approval: %s", raw)
		}
	}
	if ReferenceApproved([]byte(`{"consistency_review":"rejected"}`)) {
		t.Fatal("explicitly rejected reference must not be reused")
	}
}
