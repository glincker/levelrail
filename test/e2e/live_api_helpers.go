package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
)

// liveBuildEnv bundles the real Docker client, real BuildKit client, and
// real docker.Runtime every live test in this package that builds an
// actual image needs, cleanly skipping the calling test if Docker or
// BuildKit aren't reachable. Extracted here once deploy_test.go,
// compose_healthcheck_test.go, and multi_service_test.go had each grown
// their own copy of the identical skip-if-unavailable setup.
type liveBuildEnv struct {
	DockerCli   *dockerclient.Client
	BuildClient *build.Client
	Runtime     docker.Runtime
}

func newLiveBuildEnv(t *testing.T) liveBuildEnv {
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

	return liveBuildEnv{DockerCli: dockerCli, BuildClient: buildClient, Runtime: runtime}
}

// discardTestWriter throws away a router's own request logging, the
// same discard-writer shape test/e2e/metrics_test.go's own
// discardMetricsWriter already establishes in this package (kept
// separate rather than shared, since each live test file in this
// package is meant to be readable standalone).
type discardTestWriter struct{}

func (discardTestWriter) Write(p []byte) (int, error) { return len(p), nil }

func discardTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardTestWriter{}, nil))
}

// newE2ETestServer starts a real HTTP server for router over a real TCP
// connection on 127.0.0.1. Plain HTTP, not TLS: net/http/cookiejar
// treats a loopback origin as secure regardless of scheme (see
// net/http/cookiejar's secureMatch), so the Secure-flagged session
// cookie handleLogin sets still round-trips correctly, the same
// assumption test/e2e/metrics_test.go's TestMetrics_Live_ContainerToHTTP
// already relies on.
func newE2ETestServer(t *testing.T, router *api.Router) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(router.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// postJSON issues a real POST with a JSON body through client and
// returns the real status code and response body. Shared by every live
// test in this package that drives a real *api.Router over real HTTP
// (protected_environment_test.go, master_key_rotation_test.go), the
// same request/response pair auth_lifecycle_test.go's requestJSON
// already asserts against, generalized here to also cover POST bodies.
func postJSON(t *testing.T, client *http.Client, url, body string) (status int, respBody string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(body))) //nolint:noctx // test helper, url is loopback-only
	if err != nil {
		t.Fatalf("NewRequest(POST %s) error = %v", url, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("closing response body: %v", closeErr)
		}
	}()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	return resp.StatusCode, string(b)
}

// e2eHTTPTimeout is the shared client timeout every live *api.Router
// test in this package uses for real HTTP calls against a loopback
// server.
const e2eHTTPTimeout = 10 * time.Second

// removeContainerAndVolumes force-removes the named container along with
// its anonymous volumes (docker.Runtime.Remove leaves those behind), then
// deletes any extra named volumes. Every step ignores "not found", so it is
// safe to call before a run and again from t.Cleanup.
func removeContainerAndVolumes(cli *dockerclient.Client, containerName string, namedVolumes ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = cli.ContainerRemove(ctx, containerName, container.RemoveOptions{Force: true, RemoveVolumes: true})
	for _, name := range namedVolumes {
		_ = cli.VolumeRemove(ctx, name, true)
	}
}

// reconcileUntilAppReady calls ctrl.Reconcile repeatedly until it
// reports a True Ready condition or overallTimeout elapses. A single
// Reconcile call isn't enough for a dependent service started right
// after its own dependency: depends_on only orders container start, it
// never waits for that dependency's own readiness (internal/compose's
// documented limitation), so a db-backed app can exit once on its first
// connection attempt and only recover on the next pass, the same resync
// shape reconcile.Engine applies in production.
func reconcileUntilAppReady(parent context.Context, ctrl *application.Controller, overallTimeout time.Duration) (reconcile.Result, error) {
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
