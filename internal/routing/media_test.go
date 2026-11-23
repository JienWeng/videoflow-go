package routing

import (
	"path/filepath"
	"strings"
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

// A saved choice of a service the app no longer uses falls back to the automatic pick:
// the cheapest connected service, or fal.ai when none is connected.
func TestRetiredVideoServiceFallsBackToAutomaticChoice(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	r := Resolver{Config: config.Load(), DB: d}
	for _, name := range []string{"replicate", "kie", "wavespeed", "runway", "loopling"} {
		d.SetProviderSecret(name, "old-key", nil)
		d.SetAppSetting("default_video_provider", `"`+name+`"`)
		d.SetAppSetting("video_model", `"seedance2_fast"`)
		if p, m := r.MediaSelection("video"); p != "fal" || m != providers.FalVideoModel {
			t.Fatalf("%s with nothing connected = %s %s, want fal.ai", name, p, m)
		}
		if r.Connected(name) {
			t.Fatalf("%s still counts as connected", name)
		}
		_, _, err := r.Media(name, "seedance2_fast", "")
		if err == nil || !strings.Contains(err.Error(), "no longer available in VideoFlow; choose another video service") {
			t.Fatalf("an old %s job: %v", name, err)
		}
	}
	d.SetProviderSecret("openrouter", "key", nil)
	if p, m := r.MediaSelection("video"); p != "openrouter" || m != "bytedance/seedance-2.0" {
		t.Fatalf("with OpenRouter connected = %s %s", p, m)
	}
	d.SetProviderSecret("argolink", "key", nil)
	if p, m := r.MediaSelection("video"); p != "argolink" || m != "seedance-2.0" {
		t.Fatalf("with ArgoLink connected = %s %s, want the cheapest", p, m)
	}
}

func geminiResolver(t *testing.T, openAIKey string) (Resolver, *db.DB) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	cfg := config.Load()
	cfg.OpenAIAPIKey, cfg.GeminiAPIKey = openAIKey, ""
	return Resolver{Config: cfg, DB: d}, d
}

func TestGeminiChosenForPicturesUsesItsDefaultModel(t *testing.T) {
	r, d := geminiResolver(t, "")
	d.SetAppSetting("default_image_provider", `"gemini"`)
	for _, kind := range []string{"image", "reference"} {
		if p, m := r.MediaSelection(kind); p != "gemini" || m != providers.GeminiImageModel {
			t.Fatalf("%s: %s %s", kind, p, m)
		}
	}
	if p, _ := r.MediaSelection("video"); p == "gemini" {
		t.Fatal("Gemini picked for video")
	}
	if _, _, _, err := r.SelectedMedia("image"); err == nil || !strings.Contains(err.Error(), "gemini API key is not configured") {
		t.Fatalf("missing key error = %v", err)
	}
	d.SetProviderSecret("gemini", "stored-key", nil)
	client, provider, model, err := r.SelectedMedia("image")
	if err != nil || provider != "gemini" || model != providers.GeminiImageModel {
		t.Fatal(provider, model, err)
	}
	if _, ok := client.(*providers.GeminiImages); !ok {
		t.Fatalf("client is %T", client)
	}
	d.SetAppSetting("image_model", `"gemini-2.5-flash-image"`)
	if _, m := r.MediaSelection("image"); m != "gemini-2.5-flash-image" {
		t.Fatalf("saved Flash model not used: %s", m)
	}
}

func TestGeminiChoiceBeatsAutomaticOpenAI(t *testing.T) {
	r, d := geminiResolver(t, "openai-env-key")
	if p, _ := r.MediaSelection("image"); p != "openai" {
		t.Fatalf("an OpenAI key should still decide pictures by default, got %s", p)
	}
	d.SetAppSetting("default_image_provider", `"gemini"`)
	if p, _ := r.MediaSelection("image"); p != "gemini" {
		t.Fatalf("explicit Gemini choice ignored: %s", p)
	}
	d.SetAppSetting("default_image_provider", `"openai"`)
	if p, _ := r.MediaSelection("image"); p != "openai" {
		t.Fatalf("explicit OpenAI choice ignored: %s", p)
	}
	r.Config.OpenAIAPIKey = ""
	if p, _ := r.MediaSelection("image"); p != "fal" {
		t.Fatalf("OpenAI chosen without a key should fall back to fal, got %s", p)
	}
}

func TestGeminiEndpointIsPinned(t *testing.T) {
	r, d := geminiResolver(t, "")
	if !ValidProviderURL("gemini", providers.GeminiURL+"/") || ValidProviderURL("gemini", "https://attacker.example/v1beta") {
		t.Fatal("Gemini endpoint check is wrong")
	}
	bad := "https://attacker.example/v1beta"
	d.SetProviderSecret("gemini", "stored-key", &bad)
	if _, _, _, err := r.Credentials("gemini"); err == nil {
		t.Fatal("foreign Gemini endpoint accepted")
	}
	official := providers.GeminiURL
	d.SetProviderSecret("gemini", "stored-key", &official)
	key, endpoint, fromDB, err := r.Credentials("gemini")
	if err != nil || key != "stored-key" || endpoint != providers.GeminiURL || !fromDB {
		t.Fatal(endpoint, fromDB, err)
	}
}
