// Package testenv holds the live-Docker test helpers shared by both
// test/e2e (tests that exercise a real api.Router over HTTP) and
// test/e2e/reconcile (tests that drive the reconciler/Docker runtime
// directly, with no internal/api dependency). Splitting these out keeps
// internal/api out of test/e2e/reconcile's import graph, so a PR that
// only touches internal/api no longer selects that package too.
package testenv

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// LiveBuildEnv bundles the real Docker client, real BuildKit client, and
// real docker.Runtime every live test that builds an actual image needs,
// cleanly skipping the calling test if Docker or BuildKit aren't
// reachable.
type LiveBuildEnv struct {
	DockerCli   *dockerclient.Client
	BuildClient *build.Client
	Runtime     docker.Runtime
}

// NewLiveBuildEnv returns a LiveBuildEnv, skipping t if Docker or
// BuildKit are not reachable.
func NewLiveBuildEnv(t *testing.T) LiveBuildEnv {
	t.Helper()

	dockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	t.Cleanup(func() { _ = dockerCli.Close() })

	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, err = dockerCli.Ping(pingCtx)
	cancel()
	if err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	connectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	buildClient, err := build.NewClient(connectCtx, dockerCli)
	cancel()
	if err != nil {
		t.Skipf("could not connect to buildkit: %v", err)
	}
	t.Cleanup(func() { _ = buildClient.Close() })

	runtime, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("closing docker.Client: %v", err)
		}
	})

	return LiveBuildEnv{DockerCli: dockerCli, BuildClient: buildClient, Runtime: runtime}
}

// ReconcileUntilAppReady calls ctrl.Reconcile repeatedly until it
// reports a True Ready condition or overallTimeout elapses. A single
// Reconcile call isn't enough for a dependent service started right
// after its own dependency: depends_on only orders container start, it
// never waits for that dependency's own readiness (internal/compose's
// documented limitation), so a db-backed app can exit once on its first
// connection attempt and only recover on the next pass, the same resync
// shape reconcile.Engine applies in production.
func ReconcileUntilAppReady(parent context.Context, ctrl *application.Controller, overallTimeout time.Duration) (reconcile.Result, error) {
	deadline := time.Now().Add(overallTimeout)
	var lastResult reconcile.Result
	var lastErr error
	for {
		lastResult, lastErr = ctrl.Reconcile(parent)
		if lastErr == nil && len(lastResult.Conditions) > 0 && lastResult.Conditions[0].Status == reconcile.ConditionTrue {
			return lastResult, nil
		}
		if time.Now().After(deadline) {
			return lastResult, fmt.Errorf("did not reach a True Ready condition within %s: last error = %v, last result = %+v", overallTimeout, lastErr, lastResult)
		}
		select {
		case <-parent.Done():
			return lastResult, parent.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// OpenLiveStore opens a fresh SQLite store in a temp dir, closed on
// t.Cleanup.
func OpenLiveStore(t *testing.T) *store.DB {
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

// CleanupContainers stops and removes every container whose name has
// serviceName as a prefix, ignoring errors so it is safe to call before
// a run and again from t.Cleanup.
func CleanupContainers(ctx context.Context, t *testing.T, rt docker.Runtime, serviceName string) {
	t.Helper()
	found, err := rt.ListByPrefix(ctx, serviceName+"-")
	if err != nil {
		return
	}
	for _, cs := range found {
		_ = rt.Stop(ctx, cs.ID, 3*time.Second)
		_ = rt.Remove(ctx, cs.ID, true)
	}
}

// FreePort asks the OS for an unused TCP port on 127.0.0.1. There's an
// inherent (tiny) race between releasing the probe port here and
// whatever binds it a few lines later, but this is the standard way to
// get an ephemeral port for a test.
func FreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("FreePort: listen: %v", err)
	}
	defer func() {
		if err := l.Close(); err != nil {
			t.Logf("FreePort: closing probe listener: %v", err)
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

// GetBodyWithRetry issues GET requests against url until one succeeds
// or the deadline passes. Caddy's listeners come up asynchronously, and
// the internal-TLS case also has to finish a synchronous certificate
// issuance during Provision, so a short retry loop here is
// standalone-test plumbing.
func GetBodyWithRetry(t *testing.T, client *http.Client, url string) string {
	t.Helper()

	deadline := time.Now().Add(8 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		body, status, err := DoGet(client, url)
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

// DoGet issues a GET and returns the response body, status, and any
// error, closing the response body before returning.
func DoGet(client *http.Client, url string) (body string, status int, err error) {
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

// PersistReadyCondition writes conditions for controllerName directly,
// standing in for the production reconcile.Engine's own propagation.
// These tests drive application.Controller and ingress.Controller
// directly instead of through the Engine, so without this call the
// ingress controller's own readiness gate (F-002) never sees the
// application controller's Ready condition and refuses to route,
// exactly the failure this fix exists to prevent for a real unready
// deploy.
func PersistReadyCondition(ctx context.Context, t *testing.T, svcStore *store.DB, controllerName string, conditions []reconcile.Condition) {
	t.Helper()
	if err := svcStore.UpsertConditions(ctx, controllerName, conditions); err != nil {
		t.Fatalf("UpsertConditions() error = %v", err)
	}
}

// AssertContainerAbsent fails t unless containerName no longer exists.
func AssertContainerAbsent(ctx context.Context, t *testing.T, runtime docker.Runtime, containerName, label string) {
	t.Helper()

	state, err := runtime.InspectByName(ctx, containerName)
	if err != nil {
		t.Fatalf("InspectByName(%q) error = %v", containerName, err)
	}
	if state != nil {
		t.Fatalf("container %s (%s) still exists after being superseded, want it removed by removeStale: %+v", label, containerName, state)
	}
}

// SignHMAC returns the hex-encoded HMAC-SHA256 of payload under secret,
// the signature shape every webhook provider under test here uses.
func SignHMAC(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// WriteFile writes content to dir/name with 0o600 permissions.
func WriteFile(dir, name, content string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
}
