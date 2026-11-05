package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
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

// perfWallFactor widens the wall-clock budget into an order-of-magnitude
// tripwire; the exact guards are the query counts and plans below.
const perfWallFactor = 5

// countingTelemetry counts every telemetry read the handlers issue, each
// of which is exactly one SQL statement against metric_samples.
type countingTelemetry struct {
	TelemetryQuerier
	calls atomic.Int64
}

func (c *countingTelemetry) QueryMetrics(ctx context.Context, resourceID, metric string, from, to time.Time) ([]telemetry.Sample, error) {
	c.calls.Add(1)
	return c.TelemetryQuerier.QueryMetrics(ctx, resourceID, metric, from, to)
}

func (c *countingTelemetry) QueryLogs(ctx context.Context, resourceID string, from, to time.Time, query string) ([]telemetry.LogEntry, error) {
	c.calls.Add(1)
	return c.TelemetryQuerier.QueryLogs(ctx, resourceID, from, to, query)
}

func (c *countingTelemetry) LatestByMetric(ctx context.Context, metric string) ([]telemetry.Sample, error) {
	c.calls.Add(1)
	return c.TelemetryQuerier.LatestByMetric(ctx, metric)
}

type dashboardPerfFixture struct {
	rt     *Router
	cookie *http.Cookie
	tdb    *telemetry.DB
	tel    *countingTelemetry
	apps   int
}

// waitDiskPrewarm blocks on the boot-time docker disk usage fetch so its
// fake latency never lands inside a measured request.
func waitDiskPrewarm(t *testing.T, rt *Router) {
	t.Helper()
	c, ok := rt.dockerDiskUsage.(*cachedDiskUsager)
	if !ok {
		return
	}
	c.mu.Lock()
	flight := c.flight
	c.mu.Unlock()
	if flight == nil {
		return
	}
	select {
	case <-flight:
	case <-time.After(30 * time.Second):
		t.Fatal("docker disk usage prewarm did not finish")
	}
}

func newDashboardPerfFixture(t *testing.T) *dashboardPerfFixture {
	t.Helper()
	apps := perfEnvInt("APP_PERF_APPS", 50)
	nodes := 4
	// The endpoints read 1h windows plus latest-per-metric, so one day of raw
	// history exercises the same plans as three.
	rawDays := perfEnvInt("APP_PERF_RAW_DAYS", 1)
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
	resources := make([]string, 0, apps)
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

	tel := &countingTelemetry{TelemetryQuerier: telemetry.NewLocalFederator(tdb)}
	rt := withOfflineDoctorNetwork(NewRouter(discardLogger(), testBrand(), sdb,
		WithTelemetryQuerier(tel),
		WithDataDir(t.TempDir()),
		WithDockerPinger(slowDockerFake{ping: 5 * time.Millisecond}),
		WithDockerDiskUsager(slowDockerFake{disk: 400 * time.Millisecond}),
		WithDBPinger(sdb),
		WithDashboardCache(5*time.Second, time.Minute),
	))
	waitDiskPrewarm(t, rt)
	return &dashboardPerfFixture{rt: rt, cookie: loginTestSession(t, rt, sdb), tdb: tdb, tel: tel, apps: apps}
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

// seedPerfTelemetry writes everything in one transaction on one connection
// with synchronous off (seed only, restored after), in ascending key order.
func seedPerfTelemetry(t *testing.T, tdb *telemetry.DB, resources []string, nodes, rawDays int, now time.Time) {
	t.Helper()
	ctx := context.Background()
	conn, err := tdb.Conn(ctx)
	if err != nil {
		t.Fatalf("seed conn: %v", err)
	}
	defer func() { _ = conn.Close() }()
	// The ts index is rebuilt afterwards: maintaining it row by row while
	// every series rewrites the same ts range is the slowest part of seeding.
	for _, stmt := range []string{`PRAGMA synchronous = OFF`, `PRAGMA cache_size = -262144`, `DROP INDEX idx_metric_samples_ts`} {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed setup %q: %v", stmt, err)
		}
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("seed begin: %v", err)
	}
	steps := rawDays * 24 * 240
	gauge := []string{"cpu_percent", "memory_usage_bytes", "memory_limit_bytes", "network_rx_bytes", "network_tx_bytes"}
	nodeMetrics := []string{telemetry.MetricMemoryTotalBytes, telemetry.MetricDiskUsedBytes, telemetry.MetricDiskTotalBytes}
	sort.Strings(nodeMetrics)
	reqMetrics := perfRequestMetrics()
	sort.Strings(reqMetrics)
	for i := 0; i < nodes; i++ {
		for _, m := range nodeMetrics {
			insertPerfSeries(t, tx, fmt.Sprintf("node:node-%d", i), m, steps, 15, now)
		}
	}
	for _, res := range resources {
		for _, m := range gauge {
			insertPerfSeries(t, tx, res, m, steps, 15, now)
		}
		for _, m := range reqMetrics {
			insertPerfSeries(t, tx, res, m, 240, 15, now)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("seed commit: %v", err)
	}
	for _, stmt := range []string{`CREATE INDEX idx_metric_samples_ts ON metric_samples (ts)`, `PRAGMA synchronous = FULL`, `PRAGMA cache_size = -2000`, `ANALYZE metric_samples`} {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed finish %q: %v", stmt, err)
		}
	}
}

func insertPerfSeries(t *testing.T, tx *sql.Tx, res, metric string, n, stepSec int, now time.Time) {
	t.Helper()
	first := now.Unix() - int64(n-1)*int64(stepSec)
	_, err := tx.ExecContext(context.Background(), `
		WITH RECURSIVE s(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM s WHERE i < ?)
		INSERT INTO metric_samples (resource_id, metric, ts, value)
		SELECT ?, ?, ? + i * ?, (i % 97) + 1.0 FROM s
	`, n-1, res, metric, first, stepSec)
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

// perfQueryCeiling is the most telemetry statements one live request may
// issue per endpoint (measured, with slack). A new per-app loop would blow
// past it, which is the regression this guards.
func perfQueryCeiling(path string, apps int) int64 {
	switch path {
	case "/api/v1/apps-metrics":
		// Two overview reads (current and previous window) per app and request
		// metric, plus the latest-usage reads.
		return int64(2*apps*len(perfRequestMetrics()) + 50)
	case "/api/v1/nodes/resource-usage", "/api/v1/apps/resource-usage":
		return 10
	}
	return 0
}

// TestDashboardEndpointBudget guards that dashboard endpoints stay cheap:
// a fixed statement count per request, primary key plans for the telemetry
// reads, and a generous wall-clock tripwire (perfWallFactor x the budget,
// default 300ms) with the dashboard cache off (worst case) and on.
func TestDashboardEndpointBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds a large telemetry fixture")
	}
	f := newDashboardPerfFixture(t)
	f.assertPrimaryKeyPlans(t)
	budget := time.Duration(perfEnvInt("APP_PERF_BUDGET_MS", 300)) * time.Millisecond * perfWallFactor

	for _, path := range dashboardPerfEndpoints {
		f.rt.dash = newDashboardCache(0, 0)
		before := f.tel.calls.Load()
		live := f.get(t, path)
		queries := f.tel.calls.Load() - before
		f.rt.dash = newDashboardCache(5*time.Second, time.Minute)
		miss := f.get(t, path)
		hit := f.get(t, path)
		t.Logf("%-32s queries=%d live=%v cache_miss=%v cache_hit=%v", path, queries, live.Round(time.Millisecond), miss.Round(time.Millisecond), hit.Round(time.Millisecond))
		if ceiling := perfQueryCeiling(path, f.apps); queries > ceiling {
			t.Errorf("%s issued %d telemetry queries, ceiling %d", path, queries, ceiling)
		}
		// Doctor's first run also execs host tools (firewall probes), so its
		// budget applies to the cache fill with the network faked offline.
		measured := live
		if path == "/api/v1/system/doctor" {
			measured = miss
		}
		if measured > budget {
			t.Errorf("%s = %v, wall tripwire %v", path, measured, budget)
		}
	}
}

// assertPrimaryKeyPlans fails when a telemetry read stops seeking by the
// (resource_id, metric, ts) primary key, the property that keeps it flat as
// retention grows.
func (f *dashboardPerfFixture) assertPrimaryKeyPlans(t *testing.T) {
	t.Helper()
	queries := map[string]string{
		"range": `SELECT ts, value FROM metric_samples
			WHERE resource_id = 'service:app-00' AND metric = 'cpu_percent' AND ts BETWEEN 1 AND 9999999999 ORDER BY ts ASC`,
		"latest": `SELECT m.resource_id, m.ts, m.value FROM metric_samples m
			WHERE m.resource_id = 'service:app-00' AND m.metric = 'cpu_percent'
			AND m.ts = (SELECT MAX(ts) FROM metric_samples WHERE resource_id = 'service:app-00' AND metric = 'cpu_percent')`,
	}
	for name, q := range queries {
		rows, err := f.tdb.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+q)
		if err != nil {
			t.Fatalf("explain %s: %v", name, err)
		}
		var plan strings.Builder
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				_ = rows.Close()
				t.Fatalf("scan plan %s: %v", name, err)
			}
			plan.WriteString(detail + "\n")
		}
		_ = rows.Close()
		got := plan.String()
		if !strings.Contains(got, "USING PRIMARY KEY") || strings.Contains(got, "SCAN metric_samples") {
			t.Errorf("%s plan does not seek the primary key:\n%s", name, got)
		}
	}
}
