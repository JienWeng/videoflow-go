package api

import (
	"encoding/json"
	"strings"
	"testing"

	"videoflow-go/internal/models"
)

func TestCheapestRoutingPicksLowestPricedConnectedService(t *testing.T) {
	s := routerServer(t, fakeChatModel(`{}`))
	s.cfg.OpenRouterAPIKey = "" // start with no video service connected
	if e := s.database.SetProviderSecret("openrouter", "", nil); e != nil {
		t.Fatal(e)
	}
	type row struct {
		Provider  string
		PerSecond float64 `json:"per_second"`
		Connected bool
		Usable    bool
		Cheapest  bool
	}
	read := func(body []byte) (rows []row, current string, mode string) {
		var out struct {
			Services []row
			Routing  string
			Current  struct{ Provider, Model string }
		}
		if e := json.Unmarshal(body, &out); e != nil {
			t.Fatal(e)
		}
		return out.Services, out.Current.Provider + " " + out.Current.Model, out.Routing
	}
	rows, _, mode := read(call(t, s, "GET", "/video-services?model=seedance-2.0&resolution=720p", "").Body.Bytes())
	if len(rows) != 4 || rows[0].Provider != "argolink" || mode != "manual" {
		t.Fatalf("list = %v mode = %s", rows, mode)
	}
	for _, r := range rows {
		if r.Usable {
			t.Fatalf("nothing is connected yet, but %s is usable", r.Provider)
		}
	}
	// Only the services that list a model are shown for it.
	if rows, _, _ := read(call(t, s, "GET", "/video-services?model=seedance-2.5-flash&resolution=720p", "").Body.Bytes()); len(rows) != 2 || rows[0].Provider != "argolink" || rows[1].Provider != "openrouter" {
		t.Fatalf("2.5 Flash list = %v", rows)
	}
	// Connect two services; the cheaper of them must win, not the cheapest overall.
	for _, name := range []string{"fal", "byteplus"} {
		if w := call(t, s, "PUT", "/settings/providers/"+name, `{"api_key":"k-`+name+`"}`); w.Code != 200 {
			t.Fatalf("save %s key: %d %s", name, w.Code, w.Body.String())
		}
	}
	w := call(t, s, "PUT", "/video-services/routing", `{"mode":"cheapest","model":"seedance-2.0"}`)
	if w.Code != 200 {
		t.Fatalf("routing: %d %s", w.Code, w.Body.String())
	}
	rows, current, mode := read(w.Body.Bytes())
	if mode != "cheapest" || current != "byteplus dreamina-seedance-2-0-260128" {
		t.Fatalf("mode = %s, in use = %q; want byteplus", mode, current)
	}
	for _, r := range rows {
		if r.Cheapest != (r.Provider == "byteplus") {
			t.Fatalf("cheapest flag wrong on %v", r)
		}
	}
	if provider, model := s.routes().MediaSelection("video"); provider != "byteplus" || model != "dreamina-seedance-2-0-260128" {
		t.Fatalf("renders would use %s %s", provider, model)
	}
	// Changing the model re-runs the comparison for that model.
	call(t, s, "PUT", "/video-services/routing", `{"mode":"cheapest","model":"seedance-2.0-fast"}`)
	if provider, model := s.routes().MediaSelection("video"); provider != "byteplus" || model != "dreamina-seedance-2-0-fast-260128" {
		t.Fatalf("fast model would use %s %s", provider, model)
	}
	// Pinning one service by hand turns automatic picking off.
	if w := call(t, s, "PUT", "/video-services/routing", `{"mode":"manual","model":"seedance-2.0","provider":"fal"}`); w.Code != 200 {
		t.Fatalf("manual: %d %s", w.Code, w.Body.String())
	}
	if provider, model := s.routes().MediaSelection("video"); provider != "fal" || model != "bytedance/seedance-2.0/image-to-video" {
		t.Fatalf("manual pick gave %s %s", provider, model)
	}
	if w := call(t, s, "PUT", "/video-services/routing", `{"mode":"manual","model":"seedance-2.0","provider":"argolink"}`); w.Code != 400 {
		t.Fatalf("a service with no key must be refused, got %d", w.Code)
	}
}

// Data saved while the app still used a service it has since dropped keeps working: the
// saved choice falls back to the automatic pick, old records keep a readable name, the
// stored key is no longer read or shown, and making a video with it says what to do.
func TestRetiredVideoServiceIsNotUsedButKeepsItsName(t *testing.T) {
	s := routerServer(t, fakeChatModel(`{}`))
	s.database.SetProviderSecret("runway", "old-runway-key", nil)
	s.database.SetAppSetting("default_video_provider", `"runway"`)
	s.database.SetAppSetting("video_model", `"seedance2"`)

	var app map[string]any
	if w := call(t, s, "GET", "/settings/app", ""); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &app) != nil || app["default_video_provider"] != "openrouter" || app["video_model"] != "bytedance/seedance-2.0" {
		t.Fatalf("settings with a retired choice: %d provider=%v model=%v", w.Code, app["default_video_provider"], app["video_model"])
	}
	if w := call(t, s, "GET", "/video-services?model=seedance-2.0", ""); w.Code != 200 || strings.Contains(w.Body.String(), "runway") {
		t.Fatalf("video services: %d %s", w.Code, w.Body.String())
	}
	if w := call(t, s, "GET", "/settings/providers", ""); w.Code != 200 || strings.Contains(strings.ToLower(w.Body.String()), "runway") {
		t.Fatalf("the retired service is still listed for a key: %s", w.Body.String())
	}
	if w := call(t, s, "GET", "/settings/providers/runway", ""); strings.Contains(w.Body.String(), `"configured":true`) || strings.Contains(w.Body.String(), "-key") {
		t.Fatalf("the stored key of a retired service is still read: %s", w.Body.String())
	}
	for _, request := range [][2]string{
		{"/settings/app", `{"default_video_provider":"runway"}`},
		{"/settings/providers/runway", `{"api_key":"new"}`},
		{"/video-services/routing", `{"mode":"manual","model":"seedance-2.0","provider":"runway"}`},
	} {
		if w := call(t, s, "PUT", request[0], request[1]); w.Code != 400 {
			t.Errorf("PUT %s %s = %d, want 400", request[0], request[1], w.Code)
		}
	}
	// An old job is shown under the service's name, and making it again says what to do.
	for name, label := range map[string]string{"runway": "Runway", "kie": "Kie.ai", "replicate": "Replicate", "wavespeed": "WaveSpeed", "loopling": "Loopling"} {
		if got := videoServiceLabel(name); got != label {
			t.Errorf("videoServiceLabel(%s) = %q, want %q", name, got, label)
		}
	}
	err := s.prepareVideoJob(&models.RenderJob{Provider: "runway", Model: "seedance2"})
	if err == nil || err.Error() != "Runway is no longer available in VideoFlow; choose another video service" {
		t.Fatalf("an old Runway job made again: %v", err)
	}
}
