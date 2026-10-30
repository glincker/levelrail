package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// The dashboard load fires these endpoints in parallel. Run with:
//
//	go test ./internal/api -run TestDashboardEndpointBudget -v
//
// APP_PERF_APPS, APP_PERF_RAW_DAYS and APP_PERF_BUDGET_MS tune the fixture.
var dashboardPerfEndpoints = []string{
	"/api/v1/apps-metrics",
	"/api/v1/nodes/resource-usage",
	"/api/v1/apps/resource-usage",
	"/api/v1/system/status",
	"/api/v1/system/doctor",
}

type slowDockerFake struct {
	ping, disk time.Duration
}

func (f slowDockerFake) Ping(context.Context) error {
	time.Sleep(f.ping)
	return nil
}

func (f slowDockerFake) DiskUsage(context.Context) (docker.DiskUsage, error) {
	time.Sleep(f.disk)
	return docker.DiskUsage{}, nil
}

func perfEnvInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}

type dashboardPerfFixture struct {
	rt     *Router
	cookie *http.Cookie
}

func newDashboardPerfFixture(t *testing.T) *dashboardPerfFixture {
	t.Helper()
	apps := perfEnvInt("APP_PERF_APPS", 50)
	dbs := 10
	nodes := 4
	rawDays := perfEnvInt("APP_PERF_RAW_DAYS", 3)
	ctx := context.Background()

	sdb := openTestDB(t)
	tdb := newTestTelemetryDB(t)

	now := time.Now().UTC()
	for i := 0; i < nodes; i++ {
		n := store.Node{ID: fmt.Sprintf("node-%d", i), Name: fmt.Sprintf("node-%d", i), Address: "10.0.0.1", Status: store.NodeStatusOnline,
			AcceptsAppWorkloads: true, CreatedAt: now, UpdatedAt: now}
		if err := sdb.SaveNode(ctx, n); err != nil {
			t.Fatalf("seed node: %v", err)
		}
	}
	resources := make([]string, 0, apps+dbs)
	for i := 0; i < apps; i++ {
		name := fmt.Sprintf("app-%02d", i)
		svc := store.DesiredService{Name: name, Image: "img:1", Port: 3000, NodeID: fmt.Sprintf("node-%d", i%nodes)}
		if err := sdb.SaveDesiredService(ctx, svc); err != nil {
			t.Fatalf("seed app: %v", err)
		}
		resources = append(resources, "service:"+name)
		for j := 0; j < 20; j++ {
			id, err := store.NewDeployAttemptID()
			if err != nil {
				t.Fatalf("attempt id: %v", err)
			}
			fin := now.Add(-time.Duration(j) * time.Hour)
			a := store.DeployAttempt{ID: id, ServiceName: name, Image: "img:1", Source: store.DeployAttemptSourceImage,
				Status: store.DeployAttemptStatusSucceeded, StartedAt: fin.Add(-time.Minute), FinishedAt: &fin,
				Snapshot: store.NewDeployAttemptSnapshot(svc)}
			if err := sdb.SaveDeployAttempt(ctx, a); err != nil {
				t.Fatalf("seed attempt: %v", err)
			}
		}
	}
	seedPerfTelemetry(t, tdb, resources, nodes, rawDays, now)

	rt := withOfflineDoctorNetwork(NewRouter(discardLogger(), testBrand(), sdb,
		WithTelemetryQuerier(telemetry.NewLocalFederator(tdb)),
		WithDataDir(t.TempDir()),
		WithDockerPinger(slowDockerFake{ping: 5 * time.Millisecond}),
		WithDockerDiskUsager(slowDockerFake{disk: 400 * time.Millisecond}),
		WithDBPinger(sdb),
		WithDashboardCache(5*time.Second, time.Minute),
	))
	return &dashboardPerfFixture{rt: rt, cookie: loginTestSession(t, rt, sdb)}
}

func perfRequestMetrics() []string {
	out := []string{
		telemetry.MetricHTTPRequests, telemetry.MetricHTTPResponses4xx, telemetry.MetricHTTPResponses5xx,
		telemetry.MetricHTTPUpstreamErrors, telemetry.MetricHTTPBytesIn, telemetry.MetricHTTPBytesOut,
	}
	for i := 0; i < telemetry.LatencyBucketCount; i++ {
		out = append(out, telemetry.LatencyBucketMetric(i))
	}
	return out
}

func seedPerfTelemetry(t *testing.T, tdb *telemetry.DB, resources []string, nodes, rawDays int, now time.Time) {
	t.Helper()
	ctx := context.Background()
	steps := rawDays * 24 * 240
	gauge := []string{"cpu_percent", "memory_usage_bytes", "memory_limit_bytes", "network_rx_bytes", "network_tx_bytes"}
	for _, res := range resources {
		for _, m := range gauge {
			insertPerfSeries(t, tdb, res, m, steps, 15, now)
		}
		for _, m := range perfRequestMetrics() {
			insertPerfSeries(t, tdb, res, m, 240, 15, now)
		}
	}
	for i := 0; i < nodes; i++ {
		for _, m := range []string{telemetry.MetricMemoryTotalBytes, telemetry.MetricDiskUsedBytes, telemetry.MetricDiskTotalBytes} {
			insertPerfSeries(t, tdb, fmt.Sprintf("node:node-%d", i), m, steps, 15, now)
		}
	}
	if _, err := tdb.ExecContext(ctx, `ANALYZE`); err != nil {
		t.Fatalf("analyze: %v", err)
	}
}

func insertPerfSeries(t *testing.T, tdb *telemetry.DB, res, metric string, n, stepSec int, now time.Time) {
	t.Helper()
	_, err := tdb.ExecContext(context.Background(), `
		WITH RECURSIVE s(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM s WHERE i < ?)
		INSERT INTO metric_samples (resource_id, metric, ts, value)
		SELECT ?, ?, ? - i * ?, (i % 97) + 1.0 FROM s
	`, n-1, res, metric, now.Unix(), stepSec)
	if err != nil {
		t.Fatalf("seed series %s/%s: %v", res, metric, err)
	}
}

func (f *dashboardPerfFixture) get(t *testing.T, path string) time.Duration {
	t.Helper()
	rec := httptest.NewRecorder()
	req := authedRequest(t, f.cookie, http.MethodGet, path, "")
	start := time.Now()
	f.rt.Handler().ServeHTTP(rec, req)
	d := time.Since(start)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", path, rec.Code, rec.Body.String())
	}
	return d
}

// TestDashboardEndpointBudget times each endpoint with the dashboard cache
// off (every request does the full work, the worst case a cache miss or a
// non-dashboard caller sees) and with it on (first request after expiry,
// then a hit). Any live time over the budget (default 300ms) fails.
func TestDashboardEndpointBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds a large telemetry fixture")
	}
	f := newDashboardPerfFixture(t)
	budget := time.Duration(perfEnvInt("APP_PERF_BUDGET_MS", 300)) * time.Millisecond
	// Production prewarms the docker disk usage cache at boot; wait it out.
	time.Sleep(600 * time.Millisecond)

	for _, path := range dashboardPerfEndpoints {
		f.rt.dash = newDashboardCache(0, 0)
		live := f.get(t, path)
		f.rt.dash = newDashboardCache(5*time.Second, time.Minute)
		miss := f.get(t, path)
		hit := f.get(t, path)
		t.Logf("%-32s live=%v cache_miss=%v cache_hit=%v", path, live.Round(time.Millisecond), miss.Round(time.Millisecond), hit.Round(time.Millisecond))
		// Doctor's first run also execs host tools (firewall probes), so its
		// budget applies to the cache fill with the network faked offline.
		measured := live
		if path == "/api/v1/system/doctor" {
			measured = miss
		}
		if measured > budget {
			t.Errorf("%s = %v, budget %v", path, measured, budget)
		}
	}
}
