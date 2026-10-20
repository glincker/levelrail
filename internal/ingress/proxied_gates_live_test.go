package ingress

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// TestProxiedPath_Live_GatesHoldOnPlainListener drives the plain HTTP
// listener a fronting proxy forwards to, with the Host header it passes on.
func TestProxiedPath_Live_GatesHoldOnPlainListener(t *testing.T) {
	skipUnderLoad(t)
	hits := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte("real app"))
	}))
	defer backend.Close()
	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	// Not t.TempDir: Caddy's cert workers may still hold lock files at cleanup.
	storage, err := os.MkdirTemp("", "proxied-gates-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(storage) })
	httpPort := freePort(t)
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "proxied",
		ListenAddr: fmt.Sprintf("127.0.0.1:%d", freePort(t)),
		HTTPPort:   httpPort,
		PlainHTTP:  true,
		TLS:        true,
		StorageDir: storage,
		Routes: []ProxyRoute{{
			Hosts: []string{"private.test"}, BackendDial: backend.Listener.Addr().String(),
			BasicAuth: &BasicAuthAccount{Username: "ops", Password: string(hash)},
		}},
		MaintenanceRoutes: []MaintenanceRoute{{Hosts: []string{"down.test"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := New(testLogger(t))
	if err := d.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("apply: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop(context.Background()) })
	base := fmt.Sprintf("http://127.0.0.1:%d", httpPort)

	resp, body := doReq(t, "GET", base, "down.test", "", nil)
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body, "maintenance") {
		t.Errorf("maintenance via proxy path: status=%d body=%q", resp.StatusCode, body)
	}
	resp, _ = doReq(t, "GET", base, "private.test", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("basic auth via proxy path: status=%d, want 401", resp.StatusCode)
	}
	resp, body = doReq(t, "GET", base, "private.test", "", basic("ops", "s3cret"))
	if resp.StatusCode != http.StatusOK || body != "real app" {
		t.Errorf("authorized via proxy path: status=%d body=%q", resp.StatusCode, body)
	}
	if hits != 1 {
		t.Errorf("backend hits = %d, want 1", hits)
	}
}
