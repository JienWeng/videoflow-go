package api

import (
	"encoding/json"
	"testing"
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
	if len(rows) != 9 || rows[0].Provider != "argolink" || mode != "manual" {
		t.Fatalf("list = %v mode = %s", rows, mode)
	}
	for _, r := range rows {
		if r.Usable {
			t.Fatalf("nothing is connected yet, but %s is usable", r.Provider)
		}
	}
	// Only the services that list a model are shown for it.
	if rows, _, _ := read(call(t, s, "GET", "/video-services?model=seedance-2.5-flash&resolution=720p", "").Body.Bytes()); len(rows) != 3 || rows[0].Provider != "argolink" || rows[2].Provider != "kie" {
		t.Fatalf("2.5 Flash list = %v", rows)
	}
	// Connect two services; the cheaper of them must win, not the cheapest overall.
	for _, name := range []string{"runway", "kie"} {
		if w := call(t, s, "PUT", "/settings/providers/"+name, `{"api_key":"k-`+name+`"}`); w.Code != 200 {
			t.Fatalf("save %s key: %d %s", name, w.Code, w.Body.String())
		}
	}
	w := call(t, s, "PUT", "/video-services/routing", `{"mode":"cheapest","model":"seedance-2.0"}`)
	if w.Code != 200 {
		t.Fatalf("routing: %d %s", w.Code, w.Body.String())
	}
	rows, current, mode := read(w.Body.Bytes())
	if mode != "cheapest" || current != "kie bytedance/seedance-2" {
		t.Fatalf("mode = %s, in use = %q; want kie", mode, current)
	}
	for _, r := range rows {
		if r.Cheapest != (r.Provider == "kie") {
			t.Fatalf("cheapest flag wrong on %v", r)
		}
	}
	if provider, model := s.routes().MediaSelection("video"); provider != "kie" || model != "bytedance/seedance-2" {
		t.Fatalf("renders would use %s %s", provider, model)
	}
	// Changing the model re-runs the comparison for that model.
	call(t, s, "PUT", "/video-services/routing", `{"mode":"cheapest","model":"seedance-2.0-fast"}`)
	if provider, model := s.routes().MediaSelection("video"); provider != "kie" || model != "bytedance/seedance-2-fast" {
		t.Fatalf("fast model would use %s %s", provider, model)
	}
	// Pinning one service by hand turns automatic picking off.
	if w := call(t, s, "PUT", "/video-services/routing", `{"mode":"manual","model":"seedance-2.0","provider":"runway"}`); w.Code != 200 {
		t.Fatalf("manual: %d %s", w.Code, w.Body.String())
	}
	if provider, model := s.routes().MediaSelection("video"); provider != "runway" || model != "seedance2" {
		t.Fatalf("manual pick gave %s %s", provider, model)
	}
	if w := call(t, s, "PUT", "/video-services/routing", `{"mode":"manual","model":"seedance-2.0","provider":"argolink"}`); w.Code != 400 {
		t.Fatalf("a service with no key must be refused, got %d", w.Code)
	}
}
