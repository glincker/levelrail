package netclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOutboundTimeoutCancelsSlowServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()
	SetTimeoutSource(func() time.Duration { return 50 * time.Millisecond })
	defer SetTimeoutSource(nil)
	if _, err := New().Get(srv.URL); err == nil {
		t.Fatal("expected a timeout error")
	}
}

func TestOutboundTimeoutRefusesRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/x", http.StatusFound)
	}))
	defer srv.Close()
	resp, err := New().Get(srv.URL)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected redirect refusal")
	}
}
