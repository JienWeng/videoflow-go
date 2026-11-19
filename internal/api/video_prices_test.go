package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// Tests never read real price pages.
func init() { videoPriceSources, falPlatformURL = nil, "http://127.0.0.1:1" }

// The saved pages are real copies of each service's price source (19 Nov 2026), so a
// parser that stops matching the real layout fails here.
func TestPriceParsersReadTheRealPages(t *testing.T) {
	type page struct {
		file string
		want map[string]float64
	}
	pages := map[string]page{
		"https://openrouter.ai/api/v1/videos/models": {"openrouter.json", map[string]float64{
			"seedance-2.0": 0.007, "seedance-2.0-fast": 0.0056, "seedance-2.0-mini": 0.0035, "seedance-2.5": 0.0107, "seedance-2.5-flash": 0.007}},
		"https://loopling.ai/v1/models": {"loopling.json", map[string]float64{
			"seedance-2.0:720p": 0.13, "seedance-2.0-fast:720p": 0.104, "seedance-2.0:1080p": 0.3218, "seedance-2.5:480p": 0.0884, "seedance-2.5:720p": 0.1987, "seedance-2.5:1080p": 0.489}},
		"https://argolink.io/en/models/seedance-2.0":       {"argolink.html", map[string]float64{"seedance-2.0:480p": 0.0598, "seedance-2.0:720p": 0.129, "seedance-2.0:1080p": 0.318}},
		"https://argolink.io/en/models/seedance-2.0-fast":  {"argolink-fast.html", map[string]float64{"seedance-2.0-fast:480p": 0.0478, "seedance-2.0-fast:720p": 0.103}},
		"https://argolink.io/en/models/seedance-2.0-mini":  {"argolink-mini.html", map[string]float64{"seedance-2.0-mini:480p": 0.0299, "seedance-2.0-mini:720p": 0.0643}},
		"https://argolink.io/en/models/seedance-2.5":       {"argolink-2.5.html", map[string]float64{"seedance-2.5:480p": 0.0874, "seedance-2.5:720p": 0.196, "seedance-2.5:1080p": 0.483}},
		"https://argolink.io/en/models/seedance-2.5-flash": {"argolink-2.5-flash.html", map[string]float64{"seedance-2.5-flash:480p": 0.0572, "seedance-2.5-flash:720p": 0.129}},
		// Each WaveSpeed page lays its price tables out differently; Mini's first table is per 5 seconds.
		"https://wavespeed.ai/models/bytedance/seedance-2.0/image-to-video/llms.txt": {"wavespeed.txt", map[string]float64{
			"seedance-2.0:480p": 0.108, "seedance-2.0:720p": 0.216, "seedance-2.0:1080p": 0.54}},
		"https://wavespeed.ai/models/bytedance/seedance-2.0-fast/image-to-video/llms.txt": {"wavespeed-fast.txt", map[string]float64{
			"seedance-2.0-fast:480p": 0.09, "seedance-2.0-fast:720p": 0.18}},
		"https://wavespeed.ai/models/bytedance/seedance-2.0-mini/image-to-video/llms.txt": {"wavespeed-mini.txt", map[string]float64{
			"seedance-2.0-mini:480p": 0.054, "seedance-2.0-mini:720p": 0.108, "seedance-2.0-mini:1080p": 0.27}},
		"https://wavespeed.ai/models/bytedance/seedance-2.5/image-to-video/llms.txt": {"wavespeed-2.5.txt", map[string]float64{
			"seedance-2.5:480p": 0.162, "seedance-2.5:720p": 0.324, "seedance-2.5:1080p": 0.81}},
		"https://replicate.com/bytedance/seedance-2.0":      {"replicate.html", map[string]float64{"seedance-2.0:480p": 0.08, "seedance-2.0:720p": 0.18}},
		"https://replicate.com/bytedance/seedance-2.0-mini": {"replicate-mini.html", map[string]float64{"seedance-2.0-mini:480p": 0.04, "seedance-2.0-mini:720p": 0.09}},
		"https://replicate.com/bytedance/seedance-2.5":      {"replicate-2.5.html", map[string]float64{"seedance-2.5:480p": 0.1028, "seedance-2.5:720p": 0.2312}},
		"https://dev.runwayml.com/learn/seedance-2-0-api": {"runway.html", map[string]float64{
			"seedance-2.0:480p": 0.14, "seedance-2.0:720p": 0.30, "seedance-2.0:1080p": 0.40, "seedance-2.0-fast:480p": 0.11, "seedance-2.0-fast:720p": 0.24,
			"seedance-2.0-mini:480p": 0.07, "seedance-2.0-mini:720p": 0.15}},
		"https://dev.runwayml.com/learn/seedance-2-5-api": {"runway-2.5.html", map[string]float64{"seedance-2.5:480p": 0.20, "seedance-2.5:720p": 0.30, "seedance-2.5:1080p": 0.68}},
		"https://kie.ai/seedance-2-0": {"kie.html", map[string]float64{
			"seedance-2.0:480p": 0.095, "seedance-2.0:720p": 0.205, "seedance-2.0:1080p": 0.51, "seedance-2.0-fast:480p": 0.059, "seedance-2.0-fast:720p": 0.124}},
		"https://kie.ai/seedance-2-5":       {"kie-2.5.html", map[string]float64{"seedance-2.5:480p": 0.14, "seedance-2.5:720p": 0.315, "seedance-2.5:1080p": 0.79}},
		"https://kie.ai/seedance-2-0-mini":  {"kie-mini.html", map[string]float64{"seedance-2.0-mini:480p": 0.019, "seedance-2.0-mini:720p": 0.041}},
		"https://kie.ai/seedance-2-5-flash": {"kie-2.5-flash.html", map[string]float64{"seedance-2.5-flash:480p": 0.0925, "seedance-2.5-flash:720p": 0.205}},
	}
	checked := 0
	for _, source := range defaultPriceSources() {
		saved, known := pages[source.URL]
		if !known {
			continue
		}
		body, err := os.ReadFile("testdata/prices/" + saved.file)
		if err != nil {
			t.Fatal(err)
		}
		got := source.Parse(string(body))
		for key, price := range saved.want {
			if diff := got[key] - price; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("%s: %s = %v, want %v (all: %v)", source.URL, key, got[key], price, got)
			}
		}
		checked++
	}
	if checked != len(pages) {
		t.Fatalf("checked %d sources, want %d", checked, len(pages))
	}
}

func TestRefreshStoresLivePricesAndKeepsOldOnesWhenASourceFails(t *testing.T) {
	s := routerServer(t, fakeChatModel(`{}`))
	page := `<p>Seedance 2.0 Resolution Credits USD 480p 14 credits per second $0.14 per second 720p 25 credits per second $0.25 per second Minimum 4 seconds</p>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if page == "" {
			w.WriteHeader(500)
			return
		}
		_, _ = w.Write([]byte(page))
	}))
	defer server.Close()
	old := videoPriceSources
	defer func() { videoPriceSources = old }()
	for _, source := range defaultPriceSources() {
		if source.URL == "https://dev.runwayml.com/learn/seedance-2-0-api" {
			source.URL = server.URL
			videoPriceSources = []priceSource{source}
		}
	}
	price := func() (float64, string) {
		for _, v := range s.videoServices(httptest.NewRequest("GET", "/video-services", nil), "seedance-2.0", "720p", 10)["services"].([]videoService) {
			if v.Provider == "runway" {
				return v.PerSecond, v.Problem
			}
		}
		return 0, ""
	}
	s.refreshVideoPrices(context.Background(), true)
	if got, problem := price(); got != 0.25 || problem != "" {
		t.Fatalf("live price = %v (%q), want 0.25", got, problem)
	}
	// The page goes down: the last price read stays, with a note that it could not be refreshed.
	page = ""
	s.refreshVideoPrices(context.Background(), true)
	if got, problem := price(); got != 0.25 || problem == "" {
		t.Fatalf("after a failed read: price = %v, problem = %q", got, problem)
	}
}
