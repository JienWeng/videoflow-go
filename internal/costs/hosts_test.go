package costs

import "testing"

func TestOffersAreCheapestFirstAndMatchPublishedPrices(t *testing.T) {
	offers := Offers("seedance-2.0", "720p", "16:9", nil)
	if len(offers) != 10 || offers[3].Provider != "openrouter" {
		t.Fatalf("want all ten services at 720p, got %d: %v", len(offers), offers)
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
	if price["byteplus"] != 0.1512 || price["fal"] != 0.3024 || price["runway"] != 0.30 {
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
		t.Fatalf("kie 10 s = %v %s", amount, basis)
	}
}
