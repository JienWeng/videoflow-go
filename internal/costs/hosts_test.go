package costs

import (
	"fmt"
	"strings"
	"testing"
)

func TestOffersAreCheapestFirstAndMatchPublishedPrices(t *testing.T) {
	offers := Offers("seedance-2.0", "720p", "16:9", nil)
	if len(offers) != 9 || offers[3].Provider != "openrouter" {
		t.Fatalf("want all nine services at 720p, got %d: %v", len(offers), offers)
	}
	for i := 1; i < len(offers); i++ {
		if offers[i].PerSecond < offers[i-1].PerSecond {
			t.Fatalf("not sorted: %v", offers)
		}
	}
	if offers[0].Provider != "argolink" || offers[0].PerSecond != 0.129 {
		t.Fatalf("cheapest = %v", offers[0])
	}
	price := map[string]float64{}
	for _, o := range offers {
		price[o.Provider] = o.PerSecond
	}
	if price["byteplus"] != 0.1512 || price["fal"] != 0.3024 || price["runway"] != 0.30 || price["kie"] != 0.205 {
		t.Fatalf("token-priced services wrong: %v", price)
	}
	// A live rate from the service replaces the built-in one.
	if live := Offers("seedance-2.0-fast", "720p", "16:9", map[string]float64{"fal:seedance-2.0-fast": 0.02}); len(live) == 0 || live[len(live)-1].Provider != "fal" || live[len(live)-1].PerSecond != 0.432 {
		t.Fatalf("live price ignored: %v", live)
	}
	if FamilyOf("runway", "seedance2_fast") != "seedance-2.0-fast" || FamilyOf("fal", "bytedance/seedance-2.0/fast/text-to-video") != "seedance-2.0-fast" {
		t.Fatal("family lookup")
	}
	if amount, basis := Video("kie", "bytedance/seedance-2", "720p", "9:16", 10, nil); basis != Rate || amount != 2.05 {
		t.Fatalf("kie = %v %s", amount, basis)
	}
	if amount, _ := Video("runway", "seedance2", "720p", "9:16", 10, map[string]float64{"runway:seedance-2.0:720p": 0.25}); amount != 2.5 {
		t.Fatalf("live per-second price ignored: %v", amount)
	}
}

// A service appears for a model only when it lists that model.
func TestEachModelIsOfferedByTheServicesThatListIt(t *testing.T) {
	want := map[string]string{
		"seedance-2.0-mini":  "kie 0.041, argolink 0.0643, openrouter 0.0756, replicate 0.09, wavespeed 0.108, runway 0.15, fal 0.1512",
		"seedance-2.5":       "argolink 0.196, loopling 0.1987, openrouter 0.2311, replicate 0.2312, runway 0.3, kie 0.315, wavespeed 0.324, fal 0.4622",
		"seedance-2.5-flash": "argolink 0.129, openrouter 0.1512, kie 0.205",
	}
	for family, list := range want {
		got := []string{}
		for _, o := range Offers(family, "720p", "16:9", nil) {
			got = append(got, fmt.Sprintf("%s %v", o.Provider, o.PerSecond))
		}
		if strings.Join(got, ", ") != list {
			t.Errorf("%s at 720p:\n got %s\nwant %s", family, strings.Join(got, ", "), list)
		}
	}
	// fal has one endpoint per kind of input under each model's path.
	for model, family := range map[string]string{
		"bytedance/seedance-2.0/text-to-video": "seedance-2.0", "bytedance/seedance-2.0/fast/reference-to-video": "seedance-2.0-fast",
		"bytedance/seedance-2.0/mini/text-to-video": "seedance-2.0-mini", "bytedance/seedance-2.5/reference-to-video": "seedance-2.5", "bytedance/seedance-3/image-to-video": "",
	} {
		if got := FamilyOf("fal", model); got != family {
			t.Errorf("FamilyOf(fal, %s) = %q, want %q", model, got, family)
		}
	}
}
