package authengine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

func TestDefaultOIDCClient_RefusesInternalAddresses(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "false")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	redirector := httptest.NewServer(http.RedirectHandler("http://169.254.169.254/", http.StatusFound))
	t.Cleanup(redirector.Close)

	for _, target := range []string{srv.URL, redirector.URL, "http://169.254.169.254/", "http://10.1.2.3/", "http://192.168.0.5/"} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := defaultOIDCClient().Do(req)
		if err == nil {
			_ = resp.Body.Close()
			t.Errorf("GET %s succeeded, want refusal", target)
			continue
		}
		if !errors.Is(err, netguard.ErrBlockedAddress) {
			t.Errorf("GET %s error = %v, want ErrBlockedAddress", target, err)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("loopback server was contacted %d times, want 0", hits.Load())
	}
}
