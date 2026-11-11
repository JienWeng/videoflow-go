package routing

import (
	"path/filepath"
	"testing"
	"videoflow-go/internal/config"
	"videoflow-go/internal/db"
	"videoflow-go/internal/providers"
)

func TestFreshMediaDefaultsToFal(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	r := Resolver{Config: config.Load(), DB: d}
	p, m := r.MediaSelection("video")
	if p != "fal" || m != providers.FalVideoModel {
		t.Fatal(p, m)
	}
	p, _ = r.MediaSelection("image")
	if p != "fal" {
		t.Fatal(p)
	}
}
func TestLegacyModelOverrideMigratesToOpenRouter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.sqlite")
	d, _ := db.Open(path)
	d.SetAppSetting("video_model", `"legacy/video"`)
	d.SetAppSetting("media_provider_defaults_v1", `false`)
	d.Close()
	d, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	p, m := (Resolver{Config: config.Load(), DB: d}).MediaSelection("video")
	if p != "openrouter" || m != "legacy/video" {
		t.Fatal(p, m)
	}
}
func TestMediaCredentialsRejectForeignEndpoint(t *testing.T) {
	d, _ := db.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	defer d.Close()
	bad := "https://attacker.example/api"
	d.SetProviderSecret("fal", "private", &bad)
	if _, _, _, err := (Resolver{Config: config.Load(), DB: d}).Credentials("fal"); err == nil {
		t.Fatal("foreign fal endpoint accepted")
	}
}

func TestLegacyReferenceModelOverridePreservesOpenRouter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.sqlite")
	d, _ := db.Open(path)
	d.SetAppSetting("ref_image_model", `"legacy/reference"`)
	d.SetAppSetting("media_provider_defaults_v1", `false`)
	d.Close()
	d, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	p, m := (Resolver{Config: config.Load(), DB: d}).MediaSelection("reference")
	if p != "openrouter" || m != "legacy/reference" {
		t.Fatal(p, m)
	}
}
