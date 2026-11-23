package api

import (
	"encoding/json"
	"strings"
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

func TestAISetupChoosesGeminiAndItsModel(t *testing.T) {
	s := routerServer(t, fakeChatModel(`{}`))
	s.cfg.OpenAIAPIKey, s.cfg.GeminiAPIKey = "", ""
	pictures := func() (map[string]any, map[string]any) {
		w := call(t, s, "GET", "/ai/setup", "")
		var out map[string]map[string]any
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatalf("ai setup: %d %s", w.Code, w.Body.String())
		}
		return out["picture_service"], out["pictures"]
	}
	service, _ := pictures()
	options, _ := json.Marshal(service["options"])
	if service["provider"] == "gemini" || !strings.Contains(string(options), `"id":"gemini","label":"Google Gemini","ready":false`) {
		t.Fatalf("picture service = %v", service)
	}
	if w := call(t, s, "PUT", "/settings/app", `{"default_image_provider":"openai"}`); w.Code != 400 {
		t.Fatalf("OpenAI chosen without a key: %d", w.Code)
	}
	if w := call(t, s, "PUT", "/settings/providers/gemini", `{"api_key":"gemini-fixture"}`); w.Code != 200 || strings.Contains(w.Body.String(), "gemini-fixture") {
		t.Fatalf("save key: %d %s", w.Code, w.Body.String())
	}
	if w := call(t, s, "PUT", "/settings/app", `{"default_image_provider":"gemini"}`); w.Code != 200 {
		t.Fatalf("choose gemini: %d %s", w.Code, w.Body.String())
	}
	service, chosen := pictures()
	models, _ := json.Marshal(service["models"])
	if service["provider"] != "gemini" || service["model"] != "gemini-3-pro-image" || chosen["service"] != "Google Gemini" || chosen["ready"] != true ||
		!strings.Contains(string(models), "Nano Banana Pro") || !strings.Contains(string(models), "gemini-2.5-flash-image") {
		t.Fatalf("after choosing gemini: %v %v", service, chosen)
	}
	if w := call(t, s, "PUT", "/settings/app", `{"image_model":"gemini-2.5-flash-image","ref_image_model":"gemini-2.5-flash-image"}`); w.Code != 200 {
		t.Fatalf("choose flash: %d %s", w.Code, w.Body.String())
	}
	if w := call(t, s, "PUT", "/settings/app", `{"image_model":"gemini-3-pro-image-preview"}`); w.Code != 400 {
		t.Fatalf("retired preview model accepted: %d", w.Code)
	}
	service, chosen = pictures()
	if service["model"] != "gemini-2.5-flash-image" || chosen["note"] != "About $0.039 per picture." {
		t.Fatalf("after choosing flash: %v %v", service, chosen)
	}
	if routes := s.routes(); routes.MediaModel("reference") != "gemini-2.5-flash-image" {
		t.Fatalf("reference model = %s", routes.MediaModel("reference"))
	}
	// The same key makes videos: choosing the Gemini Omni Flash model moves video to Google.
	if w := call(t, s, "PUT", "/video-services/routing", `{"mode":"manual","model":"gemini-omni-flash"}`); w.Code != 200 {
		t.Fatalf("choose gemini video: %d %s", w.Code, w.Body.String())
	}
	w := call(t, s, "GET", "/ai/setup", "")
	var out map[string]map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatalf("ai setup: %d %s", w.Code, w.Body.String())
	}
	video := out["video"]
	if video["service"] != "Google Gemini" || video["provider"] != "gemini" || video["model"] != "gemini-omni-1.2-flash" || video["family"] != "gemini-omni-flash" || video["ready"] != true || video["per_second"] != 0.069504 {
		t.Fatalf("video = %v", video)
	}
	if out["picture_service"]["model"] != "gemini-2.5-flash-image" {
		t.Fatalf("choosing video changed the picture model: %v", out["picture_service"])
	}
	if w := call(t, s, "PUT", "/settings/app", `{"default_video_provider":"gemini","video_model":"gemini-3-pro-image"}`); w.Code != 400 {
		t.Fatalf("a picture model accepted for video: %d", w.Code)
	}
}

// The listener is OpenAI or Google Gemini: picked by hand, or by which key is saved.
func TestAISetupChoosesWhoListens(t *testing.T) {
	s := routerServer(t, fakeChatModel(`{}`))
	s.cfg.OpenAIAPIKey, s.cfg.GeminiAPIKey = "", "gemini-fixture"
	read := func() (map[string]any, map[string]any) {
		w := call(t, s, "GET", "/ai/setup", "")
		var out map[string]map[string]any
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatalf("ai setup: %d %s", w.Code, w.Body.String())
		}
		return out["listening"], out["listening_service"]
	}
	listening, service := read()
	if listening["service"] != "Google Gemini" || listening["model"] != "gemini-3.5-transcribe" || listening["ready"] != true || service["provider"] != "gemini" {
		t.Fatalf("listening = %v %v", listening, service)
	}
	options, _ := json.Marshal(service["options"])
	if string(options) != `[{"id":"openai","label":"OpenAI","ready":false},{"id":"gemini","label":"Google Gemini","ready":true}]` {
		t.Fatalf("options = %s", options)
	}
	if w := call(t, s, "PUT", "/settings/app", `{"listen_provider":"fish"}`); w.Code != 400 {
		t.Fatalf("fish accepted as listener: %d", w.Code)
	}
	if w := call(t, s, "PUT", "/settings/app", `{"listen_provider":"openai"}`); w.Code != 200 {
		t.Fatalf("choose openai: %d %s", w.Code, w.Body.String())
	}
	if listening, _ = read(); listening["service"] != "OpenAI" || listening["ready"] != false {
		t.Fatalf("after choosing OpenAI = %v", listening)
	}
}
