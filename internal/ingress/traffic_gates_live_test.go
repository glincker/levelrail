package ingress

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// gatedServer applies a config with one open host, one maintenance host and
// one basic-auth host, all pointing at the same healthy backend.
func gatedServer(t *testing.T, backendAddr, user, pass string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "gates",
		ListenAddr: addr,
		HTTPPort:   freePort(t),
		StorageDir: t.TempDir(),
		Routes: []ProxyRoute{
			{Hosts: []string{"open.test"}, BackendDial: backendAddr},
			{
				Hosts: []string{"private.test"}, BackendDial: backendAddr,
				BasicAuth: &BasicAuthAccount{Username: user, Password: string(hash)},
			},
		},
		MaintenanceRoutes: []MaintenanceRoute{{Hosts: []string{"down.test"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Apps.HTTP.Servers["gates"].AutomaticHTTPS = &AutoHTTPSConfig{Disabled: true}
	d := New(testLogger(t))
	if err := d.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("apply: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop(context.Background()) })
	return "http://" + addr
}

func basic(user, pass string) map[string]string {
	return map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass)),
	}
}

func TestMaintenance_Live_ServesFixedPageAndSkipsBackend(t *testing.T) {
	skipUnderLoad(t)
	hits := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte("real app"))
	}))
	defer backend.Close()
	base := gatedServer(t, backend.Listener.Addr().String(), "ops", "s3cret")

	resp, body := doReq(t, "GET", base, "down.test", "", nil)
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body, "maintenance") {
		t.Errorf("maintenance host: status=%d body=%q", resp.StatusCode, body)
	}
	if hits != 0 {
		t.Errorf("maintenance host reached the backend %d times", hits)
	}
	resp, body = doReq(t, "GET", base, "open.test", "", nil)
	if resp.StatusCode != http.StatusOK || body != "real app" {
		t.Errorf("open host: status=%d body=%q", resp.StatusCode, body)
	}
}

func TestBasicAuth_Live_ChallengesAndAccepts(t *testing.T) {
	skipUnderLoad(t)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("private app"))
	}))
	defer backend.Close()
	base := gatedServer(t, backend.Listener.Addr().String(), "ops", "s3cret")

	resp, body := doReq(t, "GET", base, "private.test", "", nil)
	if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("WWW-Authenticate") == "" {
		t.Errorf("no credentials: status=%d www-auth=%q", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
	}
	if strings.Contains(body, "private app") {
		t.Error("unauthenticated request received the app body")
	}
	resp, _ = doReq(t, "GET", base, "private.test", "", basic("ops", "wrong"))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong password: status=%d", resp.StatusCode)
	}
	resp, body = doReq(t, "GET", base, "private.test", "", basic("ops", "s3cret"))
	if resp.StatusCode != http.StatusOK || body != "private app" {
		t.Errorf("correct credentials: status=%d body=%q", resp.StatusCode, body)
	}
}
