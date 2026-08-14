// This file proves the one link in the observability chain that, before
// this test, had never been exercised end to end: a real running
// container's real resource usage, collected by a real
// telemetry.Collector, landing in a real telemetry store, and coming
// back out through a real internal/api.Router's HTTP handler as a real
// JSON response with plausible numbers in it.
//
// internal/telemetry/collector_live_test.go already proves container ->
// collector -> store with a real Docker daemon. internal/api/metrics_test.go
// already proves store -> HTTP handler with manually seeded samples. Ties
// exist for both halves already; what did not exist is a single test that
// walks a sample from a real container all the way to a real HTTP
// response, so this file deliberately does not re-check anything either
// of those two files already cover (it does not re-verify every metric
// name CollectOnce writes, and it does not re-check the aggregation/step
// query behavior handleQueryMetrics already has full coverage for). It
// checks exactly the seam between them.
package e2e

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// e2eMetricsAdminUsername and e2eMetricsAdminPassword are this test's own
// admin credentials, not shared with internal/api's test helpers (which
// are unexported and live in a different package): a fresh store.DB is
// bootstrapped with BootstrapAdmin (exported for exactly this reason) and
// logged in through the real handleLogin route below.
const (
	e2eMetricsAdminUsername = "e2e-metrics-admin"
	// e2eMetricsAdminPassword is a fixture credential for this test's own
	// throwaway, temp-file store.DB, never a real secret. gosec's G101
	// entropy heuristic flags this particular string (unlike
	// internal/api/testutil_test.go's shorter testAdminPassword, which
	// stays under its threshold); nolint below with this explanation per
	// CLAUDE.md 7's "no nolint without a comment" rule.
	e2eMetricsAdminPassword = "e2e-metrics-correct-horse-battery" //nolint:gosec // test fixture credential, not a real secret
)

// TestMetrics_Live_ContainerToHTTP starts a real container, runs a real
// telemetry.Collector tick against it, then makes a real HTTP request
// (over a real TCP connection via httptest.NewServer, not an in-process
// ServeHTTP call) through a real *api.Router to
// GET /api/v1/apps/{name}/metrics, and asserts the response body carries
// the container's genuine, non-zero memory usage. Skips cleanly (not a
// failure) if Docker isn't reachable, the pattern every live test in this
// repo already uses (see test/e2e/deploy_test.go,
// internal/telemetry/collector_live_test.go).
func TestMetrics_Live_ContainerToHTTP(t *testing.T) {
	dockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, err = dockerCli.Ping(pingCtx)
	cancel()
	if err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}
	if err := dockerCli.Close(); err != nil {
		t.Errorf("closing docker ping client: %v", err)
	}

	runtime, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("closing docker.Client: %v", err)
		}
	})

	// Distinct from every other e2e container/service name in this
	// package (e.g. "levelrail-test-e2e-deploy"), since sibling e2e tests
	// run concurrently against the same shared local Docker daemon.
	const name = "levelrail-test-e2e-metrics-http"
	resourceID := "service:" + name

	removeMetricsContainerIfExists(t, runtime, name)
	t.Cleanup(func() { removeMetricsContainerIfExists(t, runtime, name) })

	// A minimal image that stays running with no command of its own
	// (matching internal/telemetry/collector_live_test.go's choice, for
	// the same reason: busybox exits immediately without an explicit
	// long-running command, nginx:alpine does not). An idle container
	// still reports real non-zero memory usage and real, near-zero CPU;
	// both are legitimate "real numbers" without fabricating load.
	ctx := context.Background()
	id, err := runtime.Create(ctx, docker.ContainerSpec{Name: name, Image: "nginx:alpine"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := runtime.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Step 1: a real store, seeded with the app the metrics route looks
	// up before it will serve anything (handleQueryMetrics 404s on an app
	// it doesn't know about, see internal/api/metrics.go).
	svcStore := openMetricsLiveStore(t)
	if err := svcStore.SaveDesiredService(ctx, store.DesiredService{
		Name:  name,
		Image: "nginx:alpine",
		Port:  80,
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	// Step 2: a real telemetry.Collector, one real tick against the real
	// container, writing into a real temp-file telemetry store. This is
	// the exact wiring internal/telemetry/collector_live_test.go already
	// proves in isolation; reused here, not reinvented, as the first half
	// of the chain this test exists to connect to the second half.
	telemetryPath := filepath.Join(t.TempDir(), "telemetry.db")
	telemetryDB, err := telemetry.Open(ctx, telemetryPath)
	if err != nil {
		t.Fatalf("telemetry.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = telemetryDB.Close() })

	collector := telemetry.NewCollector(runtime, telemetryDB, time.Second, nil)
	if err := collector.CollectOnce(ctx, []telemetry.Target{
		{ResourceID: resourceID, ContainerID: id},
	}); err != nil {
		t.Fatalf("CollectOnce() error = %v", err)
	}

	// Step 3: a real *api.Router, wired to that real telemetry store via
	// api.WithTelemetryQuerier(telemetry.NewLocalFederator(...)), the same
	// construction cmd/levelrail/main.go's rootHandler uses in production
	// and internal/api/metrics_test.go's newTestRouterWithTelemetry uses
	// in that package's own tests.
	logger := slog.New(slog.NewTextHandler(discardMetricsWriter{}, nil))
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(logger, b, svcStore, api.WithTelemetryQuerier(telemetry.NewLocalFederator(telemetryDB)))

	// Step 4: a real HTTP server and a real HTTP client with a cookie
	// jar, logging in through the real POST /api/v1/auth/login route
	// (not a shortcut into the session store), matching the rigor
	// internal/api's own loginTestSession helper applies, just over a
	// real TCP connection instead of httptest.NewRecorder.
	ts := httptest.NewServer(router.Handler())
	t.Cleanup(ts.Close)

	if err := api.BootstrapAdmin(ctx, svcStore, e2eMetricsAdminUsername, e2eMetricsAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}

	loginBody := `{"username":"` + e2eMetricsAdminUsername + `","password":"` + e2eMetricsAdminPassword + `"}`
	loginResp, err := client.Post(ts.URL+"/api/v1/auth/login", "application/json", strings.NewReader(loginBody)) //nolint:noctx // test helper, ts.URL is loopback-only
	if err != nil {
		t.Fatalf("login POST error = %v", err)
	}
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want %d", loginResp.StatusCode, http.StatusOK)
	}
	if err := loginResp.Body.Close(); err != nil {
		t.Errorf("closing login response body: %v", err)
	}

	// Step 5: the real request this whole test exists to prove: a real
	// GET through the real router, real metric name and real time range
	// query params matching handleQueryMetrics' documented contract
	// (internal/api/metrics.go: metric required, from/to RFC3339).
	now := time.Now().UTC()
	url := ts.URL + "/api/v1/apps/" + name + "/metrics" +
		"?metric=memory_usage_bytes" +
		"&from=" + now.Add(-time.Hour).Format(time.RFC3339) +
		"&to=" + now.Add(time.Hour).Format(time.RFC3339)

	metricsResp, err := client.Get(url) //nolint:noctx // test helper, ts.URL is loopback-only
	if err != nil {
		t.Fatalf("metrics GET error = %v", err)
	}
	defer func() {
		if err := metricsResp.Body.Close(); err != nil {
			t.Errorf("closing metrics response body: %v", err)
		}
	}()

	if metricsResp.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d, want %d", metricsResp.StatusCode, http.StatusOK)
	}

	var got struct {
		Metric string `json:"metric"`
		Points []struct {
			Timestamp time.Time `json:"timestamp"`
			Value     float64   `json:"value"`
			Count     int       `json:"count"`
		} `json:"points"`
	}
	if err := json.NewDecoder(metricsResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}

	if got.Metric != "memory_usage_bytes" {
		t.Errorf("response metric = %q, want %q", got.Metric, "memory_usage_bytes")
	}
	if len(got.Points) != 1 {
		t.Fatalf("response points = %d, want exactly 1 (the single real sample CollectOnce wrote)", len(got.Points))
	}

	// The actual proof: a real number from a real container, not a
	// fabricated or seeded one, and not a zero that would pass a naive
	// "200 with an empty body" check. Idle nginx:alpine sits well under
	// 500MB; the lower bound rules out both a zeroed field and an
	// accidentally-empty query, the upper bound rules out an obviously
	// wrong unit or metric mix-up, without asserting an exact figure
	// that would make this test flaky across machines or nginx versions.
	const maxSaneIdleMemoryBytes = 500 * 1024 * 1024
	value := got.Points[0].Value
	if value <= 0 {
		t.Errorf("memory_usage_bytes value = %v, want a real positive figure for a running container", value)
	}
	if value >= maxSaneIdleMemoryBytes {
		t.Errorf("memory_usage_bytes value = %v, want a sane figure for an idle small container (< %d bytes)", value, maxSaneIdleMemoryBytes)
	}
	if got.Points[0].Count != 1 {
		t.Errorf("point count = %d, want 1 (one real sample)", got.Points[0].Count)
	}
}

func removeMetricsContainerIfExists(t *testing.T, rt docker.Runtime, name string) {
	t.Helper()
	state, err := rt.InspectByName(context.Background(), name)
	if err != nil || state == nil {
		return
	}
	_ = rt.Stop(context.Background(), state.ID, 3*time.Second)
	if err := rt.Remove(context.Background(), state.ID, true); err != nil {
		t.Logf("cleanup: remove %s: %v", name, err)
	}
}

func openMetricsLiveStore(t *testing.T) *store.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "levelrail.db")
	db, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test store: %v", err)
		}
	})
	return db
}

// discardMetricsWriter throws away the router's own request logging so
// test output isn't cluttered by it, matching internal/api's own
// discardWriter test helper (unexported there, so redefined here rather
// than imported).
type discardMetricsWriter struct{}

func (discardMetricsWriter) Write(p []byte) (int, error) { return len(p), nil }
