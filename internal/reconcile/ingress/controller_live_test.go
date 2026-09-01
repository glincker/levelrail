package ingress

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_Reconcile_Live is the real end-to-end proof for this
// controller, matching the rigor of every other live test in this
// codebase (see internal/reconcile/application/controller_live_test.go
// and internal/ingress/driver_test.go, which this test draws its Docker
// and Caddy plumbing from respectively): two real Docker containers
// running two different real web servers, a real Caddy instance
// configured entirely by this controller's Reconcile, and a real HTTPS
// request through Caddy for each domain, checked against each backend's
// own distinctive response body. That last part is what actually proves
// host-based routing picked the right backend, not just that some
// response came back.
//
// This is also TASKS.md 3.6's end-to-end proof for cert storage:
// WithCertStore(db), not WithStorageDir, backs Caddy's certificate/ACME
// storage here, and the assertion at the end checks internal/store's
// cert_storage table directly, proving the certificate Caddy just served
// actually came from internal/ingress.SQLiteStorage rather than a local
// file (a passing TLS handshake alone wouldn't prove that; Caddy would
// behave identically either way if the "sqlite" storage module were
// silently never wired in).
//
// Deliberately not a second, separate live test: an earlier version of
// this change added one, and two cert-issuing live Caddy configs loaded
// back-to-back in the same test binary reliably raced under `go test
// -race` inside Caddy/certmagic's own TLS background-maintenance
// goroutines (caddytls.TLS.keepStorageClean vs. its own onEvent
// callback), even though each one passed cleanly run in isolation. That
// is caddy.Load's shared package-global instance, not a bug in this
// controller or in SQLiteStorage; folding the SQLite-storage assertion
// into this single existing live test avoids it entirely.
func TestController_Reconcile_Live(t *testing.T) {
	if testing.Short() {
		t.Skip("real Docker test, skipped in short mode; see nightly.yml for the full run")
	}
	rt, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	rawCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	if _, err := rawCli.Ping(pingCtx); err != nil {
		cancel()
		t.Skipf("docker daemon not reachable: %v", err)
	}
	cancel()
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("closing docker client: %v", err)
		}
	})

	const (
		serviceA = "levelrail-test-ingress-a"
		serviceB = "levelrail-test-ingress-b"
		hostA    = "ingress-live-a.levelrail.internal"
		hostB    = "ingress-live-b.levelrail.internal"
	)
	// Two different real backends with two different, well-known default
	// response bodies, so a passing test proves domain A actually reached
	// nginx and domain B actually reached httpd, not just that Caddy
	// returned 200 for both.
	imageA := "nginx:alpine"
	imageB := "httpd:alpine"

	longCtx := context.Background()
	if err := pullIfMissing(longCtx, t, rawCli, imageA); err != nil {
		t.Fatalf("pull %s: %v", imageA, err)
	}
	if err := pullIfMissing(longCtx, t, rawCli, imageB); err != nil {
		t.Fatalf("pull %s: %v", imageB, err)
	}

	cleanupContainers(longCtx, t, rt, serviceA)
	cleanupContainers(longCtx, t, rt, serviceB)
	t.Cleanup(func() {
		cleanupContainers(context.Background(), t, rt, serviceA)
		cleanupContainers(context.Background(), t, rt, serviceB)
	})

	db := openLiveStore(t)

	desiredA := store.DesiredService{Name: serviceA, Image: imageA, Port: 80, Domains: []string{hostA}}
	desiredB := store.DesiredService{Name: serviceB, Image: imageB, Port: 80, Domains: []string{hostB}}
	if err := db.SaveDesiredService(longCtx, desiredA); err != nil {
		t.Fatalf("SaveDesiredService(a) error = %v", err)
	}
	if err := db.SaveDesiredService(longCtx, desiredB); err != nil {
		t.Fatalf("SaveDesiredService(b) error = %v", err)
	}

	startAndWaitRunning(longCtx, t, rt, application.ContainerName(serviceA, imageA, ""), imageA)
	startAndWaitRunning(longCtx, t, rt, application.ContainerName(serviceB, imageB, ""), imageB)

	caddyPort := freePort(t)
	caddyAddr := fmt.Sprintf("127.0.0.1:%d", caddyPort)
	adminPort := freePort(t)
	adminAddr := fmt.Sprintf("127.0.0.1:%d", adminPort)

	driver := ingress.New(discardLogger())
	t.Cleanup(func() {
		if err := driver.Stop(context.Background()); err != nil {
			t.Errorf("Driver.Stop() error = %v", err)
		}
	})

	ctrl := New(db, rt, driver,
		WithServerName("ingress-live"),
		WithListenAddr(caddyAddr),
		WithAdminListen(adminAddr),
		WithCertStore(db),
		WithLogger(discardLogger()),
	)

	reconcileCtx, reconcileCancel := context.WithTimeout(longCtx, 30*time.Second)
	defer reconcileCancel()
	result, err := ctrl.Reconcile(reconcileCtx)
	if err != nil {
		t.Fatalf("Reconcile() error = %v, result = %+v", err, result)
	}
	if len(result.Conditions) == 0 {
		t.Fatal("Reconcile() returned no conditions")
	}
	cond := result.Conditions[0]
	if cond.Status != "True" || cond.Reason != "Routed2Services" {
		t.Fatalf("Reconcile() condition = %+v, want Status=True Reason=Routed2Services", cond)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, caddyAddr)
			},
			// The internal issuer's root is a locally-generated CA this
			// test process never installs into a trust store, on purpose
			// (see internal/ingress/driver_test.go's identical comment):
			// only the handshake and routing are being proven here.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // deliberate, see comment above
		},
	}

	bodyA := getBodyWithRetry(t, client, "https://"+hostA+"/")
	if !strings.Contains(bodyA, "Welcome to nginx") {
		t.Errorf("response for %s = %q, want it to come from nginx (contain %q)", hostA, bodyA, "Welcome to nginx")
	}

	bodyB := getBodyWithRetry(t, client, "https://"+hostB+"/")
	if !strings.Contains(bodyB, "It works") {
		t.Errorf("response for %s = %q, want it to come from httpd (contain %q)", hostB, bodyB, "It works")
	}

	// The real proof this is host-based routing and not coincidence:
	// each domain's response must NOT match the other backend's body.
	if strings.Contains(bodyA, "It works") {
		t.Errorf("response for %s unexpectedly looked like httpd's response, routing crossed backends", hostA)
	}
	if strings.Contains(bodyB, "Welcome to nginx") {
		t.Errorf("response for %s unexpectedly looked like nginx's response, routing crossed backends", hostB)
	}

	// TASKS.md 3.6's own proof: the certificates Caddy just served both
	// domains with must actually have come from internal/store's SQLite,
	// not a local file. WithCertStore above is the only storage
	// configuration this test gives Caddy; a passing TLS handshake alone
	// wouldn't distinguish "the sqlite module worked" from "the sqlite
	// module was silently never wired in and Caddy fell back to some
	// other default."
	var storedKeys int
	if err := db.QueryRowContext(longCtx, `SELECT COUNT(*) FROM cert_storage`).Scan(&storedKeys); err != nil {
		t.Fatalf("query cert_storage row count: %v", err)
	}
	if storedKeys == 0 {
		t.Error("cert_storage has no rows after two successful HTTPS handshakes, the sqlite storage module was never actually used")
	}
}

func startAndWaitRunning(ctx context.Context, t *testing.T, rt docker.Runtime, name, image string) {
	t.Helper()

	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:  name,
		Image: image,
		Ports: []docker.PortBinding{{ContainerPort: 80}},
	})
	if err != nil {
		t.Fatalf("Create(%q) error = %v", name, err)
	}
	if err := rt.Start(ctx, id); err != nil {
		t.Fatalf("Start(%q) error = %v", name, err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		state, err := rt.InspectByName(ctx, name)
		if err != nil {
			t.Fatalf("InspectByName(%q) error = %v", name, err)
		}
		if state != nil && state.Running && len(state.Ports) > 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("container %q never reported running with a published port within the deadline", name)
}

func pullIfMissing(ctx context.Context, t *testing.T, cli *dockerclient.Client, ref string) error {
	t.Helper()
	_, err := cli.ImageInspect(ctx, ref)
	if err == nil {
		return nil
	}
	rc, err := cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	_, err = io.Copy(io.Discard, rc)
	return err
}

func cleanupContainers(ctx context.Context, t *testing.T, rt docker.Runtime, prefix string) {
	t.Helper()
	found, err := rt.ListByPrefix(ctx, prefix)
	if err != nil {
		return
	}
	for _, cs := range found {
		_ = rt.Stop(ctx, cs.ID, 3*time.Second)
		_ = rt.Remove(ctx, cs.ID, true)
	}
}

func openLiveStore(t *testing.T) *store.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "levelrail.db")
	db, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing store: %v", err)
		}
	})
	return db
}

// freePort asks the OS for an unused TCP port on 127.0.0.1, the same
// approach internal/ingress/driver_test.go uses for the identical reason:
// there's an inherent (tiny) race between releasing the probe port here
// and Caddy binding it a few lines later, but this is the standard way to
// get an ephemeral port for a test.
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

// getBodyWithRetry issues GET requests against url until one succeeds or
// the deadline passes. Caddy's listeners come up asynchronously inside
// caddy.Load, and the internal-TLS case also has to finish a synchronous
// certificate issuance during Provision, so a short retry loop here is
// standalone-test plumbing, matching internal/ingress/driver_test.go's
// identical helper.
func getBodyWithRetry(t *testing.T, client *http.Client, url string) string {
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
