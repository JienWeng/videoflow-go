package routing

import (
	"path/filepath"
	"testing"

	"videoflow-go/internal/config"
	"videoflow-go/internal/costs"
	"videoflow-go/internal/db"
	"videoflow-go/internal/providers"
)

func TestCheapestPickSkipsARejectedKey(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	r := Resolver{Config: config.Load(), DB: d}
	family := costs.Families[0].ID
	ranked := []string{}
	for _, offer := range costs.Offers(family, "720p", "16:9", nil) {
		if offer.Frames && providers.VideoHost(offer.Provider) {
			ranked = append(ranked, offer.Provider)
			if err := d.SetProviderSecret(offer.Provider, "key-"+offer.Provider, nil); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(ranked) < 2 {
		t.Fatalf("need two video services that sell %s, have %v", family, ranked)
	}
	for key, value := range map[string]string{"video_routing": `"cheapest"`, "video_family": `"` + family + `"`} {
		if err := d.SetAppSetting(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if picked, _ := r.MediaSelection("video"); picked != ranked[0] {
		t.Fatalf("cheapest pick = %s, want %s", picked, ranked[0])
	}

	// A render answered "invalid key": the service stops counting as connected at once.
	r.NoteRejectedKey(ranked[0], &providers.HTTPError{Provider: "x", Status: 401, Message: `{"code":"INVALID_API_KEY"}`})
	if r.Connected(ranked[0]) {
		t.Fatal("a rejected key still counts as connected")
	}
	if check, _ := r.KeyCheck(ranked[0]); check.State != providers.KeyRejected || check.Message == "" {
		t.Fatalf("recorded check = %+v", check)
	}
	if picked, _ := r.MediaSelection("video"); picked != ranked[1] {
		t.Fatalf("cheapest pick = %s, want the next connected service %s", picked, ranked[1])
	}

	// A newly saved key is not judged by the old key's answer.
	if err := d.SetProviderSecret(ranked[0], "a-new-key", nil); err != nil {
		t.Fatal(err)
	}
	if !r.Connected(ranked[0]) {
		t.Fatal("a new key inherited the old key's rejection")
	}
}
