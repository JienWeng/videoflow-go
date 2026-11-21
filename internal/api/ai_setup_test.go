package api

import (
	"encoding/json"
	"testing"
)

func TestAISetupReportsWhoDoesEachJobAndWhatIsMissing(t *testing.T) {
	s := routerServer(t, fakeChatModel(`{}`))
	read := func() map[string]map[string]any {
		w := call(t, s, "GET", "/ai/setup", "")
		if w.Code != 200 {
			t.Fatalf("ai setup: %d %s", w.Code, w.Body.String())
		}
		var out map[string]map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	before := read()
	for _, job := range []string{"assistant", "pictures", "video", "voice", "vision", "listening"} {
		if before[job] == nil {
			t.Fatalf("missing %s in %v", job, before)
		}
	}
	if before["voice"]["ready"] != false {
		t.Fatalf("no voice key is saved, yet voice reports ready: %v", before["voice"])
	}
	// Saving a key and choosing the service must show up as the job's default.
	call(t, s, "PUT", "/settings/providers/argolink", `{"api_key":"k"}`)
	if w := call(t, s, "PUT", "/video-services/routing", `{"mode":"manual","model":"seedance-2.0","provider":"argolink"}`); w.Code != 200 {
		t.Fatalf("choose argolink: %d %s", w.Code, w.Body.String())
	}
	// Changing only the model keeps the service and moves it to that model.
	if w := call(t, s, "PUT", "/video-services/routing", `{"mode":"manual","model":"seedance-2.0-mini"}`); w.Code != 200 {
		t.Fatalf("change model: %d %s", w.Code, w.Body.String())
	}
	video := read()["video"]
	if video["service"] != "ArgoLink" || video["family"] != "seedance-2.0-mini" || video["ready"] != true || video["per_second"] == nil {
		t.Fatalf("video = %v", video)
	}
}
