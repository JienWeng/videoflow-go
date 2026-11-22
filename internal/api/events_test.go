package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The event stream is not bound by the five-minute request limit that other requests have.
func TestEventStreamHasNoRequestDeadline(t *testing.T) {
	s := routerServer(t, nil)
	server := httptest.NewServer(s.Router())
	defer server.Close()
	res, err := http.Get(server.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 1)
		_, _ = res.Body.Read(buf)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("the stream ended on its own")
	case <-time.After(300 * time.Millisecond):
	}
}
