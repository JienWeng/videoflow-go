package db

import "testing"

func TestDisplayPromptHidesInternalIDs(t *testing.T) {
	got := displayPrompt("Pipik (char_579d07fa) waves near (asset_8ec4d7d7) towers")
	if got != "Pipik waves near towers" {
		t.Fatal(got)
	}
}
