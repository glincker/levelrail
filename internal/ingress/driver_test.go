package ingress

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// freePort asks the OS for an unused TCP port on 127.0.0.1 by binding to
// port 0, reading back what it picked, and immediately releasing it.
// There's an inherent (tiny) race between releasing the port here and
// Caddy binding it a few lines later in the caller, but this is the
// standard way to get an ephemeral port for a test and is not a concern
// specific to this package.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: listen: %v", err)
	}
	defer func() {
		if err := l.Close(); err != nil {
			t.Logf("freePort: closing probe listener: %v", err)
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

// TestDriver_ReverseProxy_PlainHTTP is the core proof for the embedded
// Caddy ingress design and this spike's task item 3: Caddy, running
// in-process (no `caddy` binary,
// no container), actually proxies a real HTTP request to a real backend.
//
// This starts three things in the same test process: a trivial backend
// (httptest.NewServer), Caddy itself (via Driver.Apply, which calls
// caddy.Load), and an http.Client making a real network request to
// Caddy's listener. The backend's fixed response string coming back
// through that request is the proof; nothing here is mocked.
func TestDriver_ReverseProxy_PlainHTTP(t *testing.T) {
	if testing.Short() {
		t.Skip("hits a known upstream Caddy v2.11.4 data race under concurrent CI load, covered by the nightly full suite")
	}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprint(w, "hello from the real backend"); err != nil {
			t.Errorf("backend: writing response: %v", err)
		}
	}))
	defer backend.Close()

	backendAddr := backend.Listener.Addr().String() // e.g. 127.0.0.1:54321
	caddyPort := freePort(t)
	caddyAddr := fmt.Sprintf("127.0.0.1:%d", caddyPort)

	cfg, err := BuildProxyConfig(ProxyOptions{
		ServerName:  "spike-http",
		ListenAddr:  caddyAddr,
		BackendDial: backendAddr,
		StorageDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("BuildProxyConfig() error: %v", err)
	}

	d := New(testLogger(t))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := d.Apply(ctx, cfg); err != nil {
		t.Fatalf("Driver.Apply() error: %v", err)
	}
	defer func() {
		if err := d.Stop(ctx); err != nil {
			t.Errorf("Driver.Stop() error: %v", err)
		}
	}()

	body := getBodyWithRetry(t, "http://"+caddyAddr+"/")
	const want = "hello from the real backend"
	if body != want {
		t.Errorf("response through Caddy = %q, want %q", body, want)
	}
}

// TestDriver_ReverseProxy_InternalTLS proves the automatic-certificate
// mechanism this spike is required to verify: Caddy's "internal" issuer
// (self-signed, fully offline, see InternalIssuer's doc comment) issues a
// certificate for a configured hostname with no operator step, and the
// resulting HTTPS listener serves real proxied traffic over it. Real
// public ACME needs a public domain and inbound 80/443 reachable from the
// internet, neither of which exists in this sandbox; see
// docs-local/research/caddy-spike.md for what still needs verifying
// against a real domain later.
func TestDriver_ReverseProxy_InternalTLS(t *testing.T) {
	if testing.Short() {
		t.Skip("hits a known upstream Caddy v2.11.4 data race under concurrent CI load, covered by the nightly full suite")
	}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprint(w, "hello over TLS from the real backend"); err != nil {
			t.Errorf("backend: writing response: %v", err)
		}
	}))
	defer backend.Close()

	backendAddr := backend.Listener.Addr().String()
	caddyPort := freePort(t)
	caddyAddr := fmt.Sprintf("127.0.0.1:%d", caddyPort)
	const host = "ingress-spike.levelrail.internal"

	cfg, err := BuildProxyConfig(ProxyOptions{
		ServerName:  "spike-tls",
		ListenAddr:  caddyAddr,
		BackendDial: backendAddr,
		Hosts:       []string{host},
		TLS:         true,
		StorageDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("BuildProxyConfig() error: %v", err)
	}

	d := New(testLogger(t))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := d.Apply(ctx, cfg); err != nil {
		t.Fatalf("Driver.Apply() error: %v", err)
	}
	defer func() {
		if err := d.Stop(ctx); err != nil {
			t.Errorf("Driver.Stop() error: %v", err)
		}
	}()

	// The request URL uses the fake hostname (not the real caddyAddr) so
	// that both the HTTP Host header and the default TLS SNI (Go's
	// http.Transport derives ServerName from the URL host when
	// TLSClientConfig.ServerName is unset) match the host our route and
	// automation policy are scoped to. DialContext is overridden to send
	// the actual TCP connection to caddyAddr instead of trying to
	// resolve "ingress-spike.levelrail.internal" in DNS, since it isn't
	// a real, resolvable name.
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, caddyAddr)
			},
			TLSClientConfig: &tls.Config{
				// The internal issuer's root is a locally-generated CA
				// this test process never installs into a trust store,
				// on purpose, that step is not part of what this spike
				// is proving. InsecureSkipVerify only disables chain
				// verification; the handshake itself, and the fact that
				// Caddy served this test a certificate for the right
				// SNI at all, is the thing being proven.
				InsecureSkipVerify: true, //nolint:gosec // deliberate: see comment above
			},
		},
	}

	body := getBodyWithRetryClient(t, client, "https://"+host+"/")
	const want = "hello over TLS from the real backend"
	if body != want {
		t.Errorf("response through Caddy over TLS = %q, want %q", body, want)
	}
}

// getBodyWithRetry issues GET requests against url until one succeeds or
// the deadline passes. Caddy's listeners come up asynchronously inside
// caddy.Load (Apply returns once the config is accepted, not once every
// listener is guaranteed accepting connections, and the internal-TLS case
// also has to finish a synchronous certificate issuance during Provision),
// so a short retry loop here is standalone-test plumbing, not a
// workaround for a Caddy bug.
func getBodyWithRetry(t *testing.T, url string) string {
	t.Helper()
	return getBodyWithRetryClient(t, http.DefaultClient, url)
}

func getBodyWithRetryClient(t *testing.T, client *http.Client, url string) string {
	t.Helper()

	deadline := time.Now().Add(8 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		body, status, err := doGet(client, url)
		if err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if status != http.StatusOK {
			lastErr = fmt.Errorf("status %d", status)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return body
	}

	t.Fatalf("GET %s never succeeded within the retry window, last error: %v", url, lastErr)
	return ""
}

// doGet issues a single GET and reads the full body, closing the response
// body itself rather than leaving that to the caller, so a retry loop
// calling this repeatedly never accumulates open response bodies.
func doGet(client *http.Client, url string) (body string, status int, err error) {
	resp, err := client.Get(url) //nolint:noctx // test helper, no context to plumb through
	if err != nil {
		return "", 0, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing response body: %w", closeErr)
		}
	}()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, fmt.Errorf("reading response body: %w", err)
	}
	return string(b), resp.StatusCode, nil
}
