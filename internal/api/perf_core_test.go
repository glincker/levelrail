package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

// perfSeed fills a store with a realistic mid-size install for handler timing.
func perfSeed(t *testing.T, db *store.DB) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 50; i++ {
		now := time.Now()
		if err := db.SaveNode(ctx, store.Node{ID: fmt.Sprintf("node-%03d", i), Name: fmt.Sprintf("node-%03d", i), Address: fmt.Sprintf("10.0.0.%d:7443", i), Status: store.NodeStatus("ready"), CertFingerprint: fmt.Sprintf("fp%03d", i), JoinedAt: &now, LastSeenAt: &now, AcceptsAppWorkloads: true}); err != nil {
			t.Fatalf("seed node: %v", err)
		}
	}
	for i := 0; i < 20; i++ {
		if err := db.SaveProject(ctx, store.Project{ID: fmt.Sprintf("prj-%03d", i), Name: fmt.Sprintf("project-%d", i), CreatedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
			t.Fatalf("seed project: %v", err)
		}
	}
	for i := 0; i < 200; i++ {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: fmt.Sprintf("app-%03d", i), Image: "img:v1", Port: 3000, ProjectID: fmt.Sprintf("prj-%03d", i%20)}); err != nil {
			t.Fatalf("seed app: %v", err)
		}
	}
	for i := 0; i < 100; i++ {
		if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: fmt.Sprintf("db-%03d", i), Engine: "postgres", Version: "16", ProjectID: fmt.Sprintf("prj-%03d", i%20)}); err != nil {
			t.Fatalf("seed db: %v", err)
		}
	}
	base := time.Now().Add(-30 * 24 * time.Hour)
	for i := 0; i < 10000; i++ {
		status := store.DeployAttemptStatusSucceeded
		if i%9 == 0 {
			status = store.DeployAttemptStatusFailed
		}
		if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: fmt.Sprintf("dpl_%06d", i), ServiceName: fmt.Sprintf("app-%03d", i%200), Image: fmt.Sprintf("img:v%d", i), Source: store.DeployAttemptSourceImage, Status: status, StartedAt: base.Add(time.Duration(i) * 4 * time.Minute), Sequence: int64(i / 200)}); err != nil {
			t.Fatalf("seed deploy: %v", err)
		}
	}
}

// TestPerfCoreBaseline times hot handlers against a seeded store. Run without -short:
// go test -run TestPerfCoreBaseline -v ./internal/api
func TestPerfCoreBaseline(t *testing.T) {
	if testing.Short() {
		t.Skip("perf baseline is slow, skipped in -short")
	}
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	perfSeed(t, db)
	h := rt.Handler()
	targets := []string{
		"/api/v1/apps", "/api/v1/apps-summary", "/api/v1/databases", "/api/v1/nodes", "/api/v1/projects",
		"/api/v1/apps/app-007/deploys", "/api/v1/deployments/summary", "/api/v1/usage/summary",
		"/api/v1/databases/db-001/explorer/capabilities",
	}
	const iters = 10
	for _, target := range targets {
		do := func() *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			req.AddCookie(cookie)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			return rec
		}
		do()
		var m0, m1 runtime.MemStats
		q0 := db.Metrics().Queries
		runtime.GC()
		runtime.ReadMemStats(&m0)
		start := time.Now()
		var rec *httptest.ResponseRecorder
		for i := 0; i < iters; i++ {
			rec = do()
		}
		el := time.Since(start)
		runtime.ReadMemStats(&m1)
		q1 := db.Metrics().Queries
		t.Logf("PERF %-50s status=%d bytes=%d ms/req=%.2f allocs/req=%d KB/req=%d sql/req=%d", target, rec.Code, rec.Body.Len(),
			float64(el.Microseconds())/1000/iters, (m1.Mallocs-m0.Mallocs)/iters, (m1.TotalAlloc-m0.TotalAlloc)/iters/1024, (q1-q0)/iters)
	}
	// Plans for the hottest statements seen so far.
	seen := map[string]bool{}
	for _, hs := range db.Metrics().HotStatements {
		if seen[hs.SQL] || !strings.HasPrefix(strings.TrimSpace(strings.ToUpper(hs.SQL)), "SELECT") {
			continue
		}
		seen[hs.SQL] = true
		rows, err := db.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+hs.SQL)
		if err != nil {
			t.Logf("PLAN-ERR %v :: %.120s", err, perfOneLine(hs.SQL))
			continue
		}
		var plan []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err == nil {
				plan = append(plan, detail)
			}
		}
		_ = rows.Close()
		t.Logf("PLAN n=%d ms=%.1f :: %.200s => %s", hs.Count, hs.TotalMS, perfOneLine(hs.SQL), strings.Join(plan, " | "))
	}
}

func perfOneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func TestDeploymentSummaryCacheKeepsVisibilityApart(t *testing.T) {
	rt, db := newTestRouter(t)
	_ = rt
	ds, ok := any(db).(deploymentStore)
	if !ok {
		t.Skip("store lacks deployment briefs")
	}
	seedApp(t, db, "a")
	since := time.Now().Add(-time.Hour)
	if _, err := cachedDeploymentBriefs(context.Background(), ds, since, nil, "", "", time.Minute); err != nil {
		t.Fatal(err)
	}
	all, _ := cachedDeploymentBriefs(context.Background(), ds, since, nil, "", "", time.Minute)
	none, _ := cachedDeploymentBriefs(context.Background(), ds, since, []string{}, "", "", time.Minute)
	if len(none) != 0 || all == nil && none == nil {
		t.Fatalf("empty visible set must not share the unfiltered entry: all=%d none=%d", len(all), len(none))
	}
}

func TestDeploymentSummaryCacheSettingDisablesCache(t *testing.T) {
	rt, _ := newTestRouter(t)
	if got := rt.deploySummaryTTL(context.Background()); got != 3*time.Second {
		t.Fatalf("default ttl = %v", got)
	}
}
