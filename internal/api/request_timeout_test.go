package api

import "testing"

func TestOnlyEventsAndLocalExportsOutlastTheRequestTimeout(t *testing.T) {
	for path, want := range map[string]bool{
		"/events":                       true,
		"/compositions/abc123/run":      true,
		"/compositions/abc123":          false,
		"/scenes/scene_1/render":        false,
		"/compositions/abc123/run/more": false,
		"/graph":                        false,
	} {
		if got := exemptFromRequestTimeout(path); got != want {
			t.Errorf("%s: exempt=%v, want %v", path, got, want)
		}
	}
}
