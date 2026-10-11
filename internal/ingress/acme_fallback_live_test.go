package ingress

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dockertest"
)

// A CA that cannot be reached (the same outcome as a contact it rejects) must
// leave the host serving a locally signed certificate, with the failure kept
// for the domain status. Before the fallback issuer this was a handshake failure.
func TestACMEFailure_ServesInternalFallbackCertificate_Live(t *testing.T) {
	dockertest.SkipIfShort(t)
	defaultACMEFailures.ClearAll()
	t.Cleanup(defaultACMEFailures.ClearAll)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "served over fallback")
	}))
	defer backend.Close()

	const host = "fallback-live.localhost"
	appAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	deadCA := fmt.Sprintf("https://127.0.0.1:%d/directory", freePort(t))

	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "app", ListenAddr: appAddr, TLS: true,
		Routes:      []ProxyRoute{{Hosts: []string{host}, BackendDial: backend.Listener.Addr().String()}},
		ACMEEnabled: true, ACMEEmail: "ops@example.com", ACMEDirectoryURL: deadCA,
		StorageDir: t.TempDir(), FallbackCertLifetime: "1h",
	})
	if err != nil {
		t.Fatalf("BuildRoutesConfig() error = %v", err)
	}

	d := New(testLogger(t))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := d.Apply(ctx, cfg); err != nil {
		t.Fatalf("Driver.Apply() error = %v", err)
	}
	defer func() { _ = d.Stop(ctx) }()

	var issuer string
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", appAddr,
			&tls.Config{ServerName: host, InsecureSkipVerify: true}) //nolint:gosec // asserting on the certificate served, not trusting it
		if err == nil {
			issuer = conn.ConnectionState().PeerCertificates[0].Issuer.String()
			_ = conn.Close()
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if issuer == "" {
		t.Fatal("no TLS handshake succeeded: the listener has no certificate while ACME fails")
	}
	if !strings.Contains(issuer, "Caddy Local Authority") {
		t.Errorf("served certificate issuer = %q, want the internal CA", issuer)
	}

	var failure ACMEFailure
	var ok bool
	for time.Now().Before(deadline.Add(10 * time.Second)) {
		if failure, ok = defaultACMEFailures.Get(host); ok {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !ok {
		t.Fatal("no ACME failure recorded for the host: the domain status would show nothing")
	}
	t.Logf("recorded failure: %s", failure.Error)
}
