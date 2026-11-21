package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeArgoLink accepts only "good-key" on its free balance request and counts every request.
func fakeArgoLink(t *testing.T) *int {
	t.Helper()
	calls := 0
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/account/balance" {
			t.Errorf("key check sent %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer good-key" {
			w.WriteHeader(401)
			w.Write([]byte(`{"code":"INVALID_API_KEY","message":"Invalid API key"}`))
			return
		}
		w.Write([]byte(`{"available":"5.00","currency":"USD"}`))
	}))
	previous := keyCheckBase
	keyCheckBase = func(string, string) string { return service.URL }
	t.Cleanup(func() { keyCheckBase = previous; service.Close() })
	return &calls
}

type keyCheckBody struct {
	Configured bool `json:"configured"`
	KeyCheck   *struct {
		State   string `json:"state"`
		Message string `json:"message"`
	} `json:"key_check"`
}

func saveKey(t *testing.T, s *Server, key string) (*httptest.ResponseRecorder, keyCheckBody) {
	t.Helper()
	result := call(t, s, "PUT", "/settings/providers/argolink", `{"api_key":"`+key+`"}`)
	var body keyCheckBody
	json.Unmarshal(result.Body.Bytes(), &body)
	return result, body
}

func configured(t *testing.T, s *Server) keyCheckBody {
	var body keyCheckBody
	json.Unmarshal(call(t, s, "GET", "/settings/providers/argolink", "").Body.Bytes(), &body)
	return body
}

func TestSavingAVideoKeyAsksTheService(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no other service may be called") })
	fakeArgoLink(t)

	result, _ := saveKey(t, s, "wrong-key")
	if result.Code != 400 || strings.TrimSpace(result.Body.String()) != "ArgoLink did not accept this key. Copy the API key from your ArgoLink dashboard and paste it again." {
		t.Fatalf("wrong key: %d %s", result.Code, result.Body.String())
	}
	result, _ = saveKey(t, s, "whsec_0123456789")
	if result.Code != 400 || !strings.Contains(result.Body.String(), "webhook signing secret, not an API key") {
		t.Fatalf("webhook secret: %d %s", result.Code, result.Body.String())
	}
	if configured(t, s).Configured {
		t.Fatal("a refused key was saved")
	}

	result, body := saveKey(t, s, "good-key")
	if result.Code != 200 || !body.Configured || body.KeyCheck == nil || body.KeyCheck.State != "accepted" {
		t.Fatalf("good key: %d %s", result.Code, result.Body.String())
	}
	test := call(t, s, "POST", "/settings/providers/argolink/test", "")
	if !strings.Contains(test.Body.String(), `"credentials_verified":true`) || !strings.Contains(test.Body.String(), `"ok":true`) {
		t.Fatalf("test of a good key: %s", test.Body.String())
	}
}

func TestAKeyThatCannotBeCheckedIsStillSaved(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no other service may be called") })
	// The package default points key checks at a closed port, as when the service is unreachable.
	result, body := saveKey(t, s, "any-key")
	if result.Code != 200 || !body.Configured || body.KeyCheck == nil || body.KeyCheck.State != "unchecked" || !strings.Contains(body.KeyCheck.Message, "could not be checked") {
		t.Fatalf("unreachable service: %d %s", result.Code, result.Body.String())
	}
}

func TestASavedKeyIsJudgedOnceADayAndRejectionShowsEverywhere(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no other service may be called") })
	calls := fakeArgoLink(t)
	// A key saved before keys were checked.
	if err := s.database.SetProviderSecret("argolink", "whsec_old", nil); err != nil {
		t.Fatal(err)
	}
	s.database.SetAppSetting("default_video_provider", `"argolink"`)
	s.database.SetAppSetting("video_model", `"seedance-2.0"`)

	list := call(t, s, "GET", "/settings/providers", "").Body.String()
	if !strings.Contains(list, `"state":"rejected"`) || !strings.Contains(list, "webhook signing secret") {
		t.Fatalf("settings list does not show the rejection: %s", list)
	}
	var setup struct {
		Video struct {
			Ready    bool   `json:"ready"`
			Rejected bool   `json:"key_rejected"`
			Note     string `json:"note"`
		} `json:"video"`
	}
	json.Unmarshal(call(t, s, "GET", "/ai/setup", "").Body.Bytes(), &setup)
	if setup.Video.Ready || !setup.Video.Rejected || setup.Video.Note == "" {
		t.Fatalf("AI setup video = %+v", setup.Video)
	}
	var services struct {
		Services []struct {
			Provider  string `json:"provider"`
			Connected bool   `json:"connected"`
			Usable    bool   `json:"usable"`
			KeyCheck  *struct {
				State string `json:"state"`
			} `json:"key_check"`
		} `json:"services"`
	}
	json.Unmarshal(call(t, s, "GET", "/video-services", "").Body.Bytes(), &services)
	found := false
	for _, row := range services.Services {
		if row.Provider == "argolink" {
			found = true
			if row.Connected || row.Usable || row.KeyCheck == nil || row.KeyCheck.State != "rejected" {
				t.Fatalf("video services row = %+v", row)
			}
		}
	}
	if !found {
		t.Fatal("ArgoLink missing from video services")
	}
	if *calls != 0 {
		t.Fatalf("a webhook secret was sent to the service %d times", *calls)
	}

	// A real key saved before checks: asked about once, not on every screen load.
	if err := s.database.SetProviderSecret("argolink", "good-key", nil); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		call(t, s, "GET", "/settings/providers", "")
		call(t, s, "GET", "/ai/setup", "")
	}
	if *calls != 1 {
		t.Fatalf("saved key checked %d times across six screen loads, want 1", *calls)
	}
	if check := configured(t, s).KeyCheck; check == nil || check.State != "accepted" {
		t.Fatalf("saved good key = %+v", check)
	}
}
