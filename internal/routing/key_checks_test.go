package routing

import (
	"path/filepath"
	"testing"

	"videoflow-go/internal/config"
	"videoflow-go/internal/costs"
	"videoflow-go/internal/db"
	"videoflow-go/internal/providers"
)

// cheapestFixture saves a key for every video service that sells the default model and
// turns on "always the cheapest". It returns those services, cheapest first.
func cheapestFixture(t *testing.T) (Resolver, *db.DB, []string) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
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
	return r, d, ranked
}

func TestCheapestPickSkipsARejectedKey(t *testing.T) {
	r, d, ranked := cheapestFixture(t)

	// A render answered "invalid key": the service stops counting as connected at once.
	r.NoteServiceAnswer(ranked[0], &providers.HTTPError{Provider: "x", Status: 401, Message: `{"code":"INVALID_API_KEY"}`})
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

func TestCheapestPickSkipsAnAccountWithNoBalance(t *testing.T) {
	for name, answer := range map[string]error{
		"fal.ai locked account": &providers.HTTPError{Provider: "x", Status: 403, Message: `{"detail":"User is locked. Reason: Exhausted balance. Top up your balance at fal.ai/dashboard/billing."}`},
		"payment required":      &providers.HTTPError{Provider: "x", Status: 402, Message: `{"error":"payment required"}`},
		"Kie.ai code 402":       &providers.HTTPError{Provider: "Kie.ai", Status: 402, Message: "Credits insufficient"},
	} {
		t.Run(name, func(t *testing.T) {
			r, _, ranked := cheapestFixture(t)
			r.NoteServiceAnswer(ranked[0], answer)
			check, _ := r.KeyCheck(ranked[0])
			if check.State != providers.KeyNoCredit || check.Message == "" {
				t.Fatalf("recorded check = %+v", check)
			}
			if r.Connected(ranked[0]) {
				t.Fatal("an account with no balance still counts as connected")
			}
			if picked, _ := r.MediaSelection("video"); picked != ranked[1] {
				t.Fatalf("cheapest pick = %s, want the next service %s", picked, ranked[1])
			}
			// Only the owner's Check again (or a normal answer) asks about it again.
			if r.KeyCheckDue(ranked[0], check.CheckedAt.AddDate(0, 0, 3)) {
				t.Fatal("the daily key check would clear an empty account by itself")
			}
			r.NoteServiceAnswer(ranked[0], nil)
			if !r.Connected(ranked[0]) {
				t.Fatal("a normal answer did not clear the empty-account state")
			}
			if picked, _ := r.MediaSelection("video"); picked != ranked[0] {
				t.Fatalf("cheapest pick = %s after the top-up, want %s", picked, ranked[0])
			}
		})
	}
}

func TestOtherFailuresLeaveTheServiceConnected(t *testing.T) {
	r, _, ranked := cheapestFixture(t)
	r.NoteServiceAnswer(ranked[0], &providers.HTTPError{Provider: "x", Status: 500, Message: "upstream timeout"})
	r.NoteServiceAnswer(ranked[0], &providers.HTTPError{Provider: "x", Status: 403, Message: "content policy"})
	if !r.Connected(ranked[0]) {
		t.Fatal("a passing failure disconnected the service")
	}
}
