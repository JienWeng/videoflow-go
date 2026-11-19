package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVideoServiceKeyCheck(t *testing.T) {
	const good = "good-key"
	paths := map[string]string{"argolink": "/account/balance", "loopling": "/me", "replicate": "/account", "kie": "/v1/chat/credit", "wavespeed": "/balance", "runway": "/organization"}
	for name, path := range paths {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.Method != "GET" || r.URL.Path != path {
				t.Errorf("%s: key check sent %s %s; only the free GET %s is allowed", name, r.Method, r.URL.Path, path)
			}
			valid := r.Header.Get("Authorization") == "Bearer "+good
			switch {
			case name == "kie" && valid:
				w.Write([]byte(`{"code":200,"msg":"success","data":12.5}`))
			case name == "kie": // Kie.ai answers HTTP 200 and puts the refusal in the body.
				w.Write([]byte(`{"code":401,"msg":"You do not have access permissions"}`))
			case valid:
				w.Write([]byte(`{}`))
			default:
				w.WriteHeader(401)
				w.Write([]byte(`{"code":"INVALID_API_KEY","message":"Invalid API key"}`))
			}
		}))
		label := NewHostClient(name, "", "").host.Label
		if got := NewHostClient(name, good, server.URL).CheckKey(context.Background()); got.State != KeyAccepted {
			t.Errorf("%s: good key = %+v", name, got)
		}
		got := NewHostClient(name, "wrong", server.URL).CheckKey(context.Background())
		if want := label + " did not accept this key. Copy the API key from your " + label + " dashboard and paste it again."; got.State != KeyRejected || got.Message != want {
			t.Errorf("%s: wrong key = %+v", name, got)
		}
		before := calls
		got = NewHostClient(name, "whsec_abc123", server.URL).CheckKey(context.Background())
		if got.State != KeyRejected || !strings.Contains(got.Message, "webhook signing secret, not an API key") || calls != before {
			t.Errorf("%s: webhook secret = %+v after %d requests", name, got, calls-before)
		}
		server.Close()
		if got := NewHostClient(name, good, server.URL).CheckKey(context.Background()); got.State != KeyUnchecked || !strings.Contains(got.Message, "could not be checked") {
			t.Errorf("%s: unreachable service = %+v", name, got)
		}
	}
}

func TestOnlyKeyProblemsCountAsInvalidKey(t *testing.T) {
	cases := map[*HTTPError]bool{
		{Status: 401, Message: `{"code":"INVALID_API_KEY","message":"Invalid API key"}`}: true,
		{Status: 403, Message: `{"error":"Invalid API key provided"}`}:                   true,
		{Status: 403, Message: `{"type":"permission_error","code":"tier_gated"}`}:        false,
		{Status: 402, Message: `{"code":"insufficient_ink"}`}:                            false,
		{Status: 500, Message: `{"message":"Unauthorized upstream"}`}:                    false,
	}
	for err, want := range cases {
		if got := InvalidKey(err); got != want {
			t.Errorf("InvalidKey(%d %s) = %v", err.Status, err.Message, got)
		}
	}
}
