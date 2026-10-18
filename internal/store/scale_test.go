package store

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
)

const (
	scaleApps     = 1000
	scaleAttempts = 5000
	scaleSamples  = 200
	scalePage     = 50
)

func scalePercentiles(d []time.Duration) (p50, p95, p99 float64) {
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	at := func(p float64) float64 {
		return float64(d[min(len(d)-1, int(p*float64(len(d))))]) / float64(time.Millisecond)
	}
	return at(0.50), at(0.95), at(0.99)
}

func scaleTime(n int, fn func(i int)) []time.Duration {
	out := make([]time.Duration, n)
	for i := range out {
		start := time.Now()
		fn(i)
		out[i] = time.Since(start)
	}
	return out
}

// TestScaleSeeded prints the numbers docs/benchmarks.md quotes. Set LEVELRAIL_SCALE_BENCH=1 to run it.
func TestScaleSeeded(t *testing.T) {
	if os.Getenv("LEVELRAIL_SCALE_BENCH") != "1" {
		t.Skip("set LEVELRAIL_SCALE_BENCH=1 to run the seeded scale measurement")
	}
	db := openTestDB(t)
	ctx := context.Background()

	seedStart := time.Now()
	names := make([]string, scaleApps)
	for i := range names {
		names[i] = fmt.Sprintf("app-%04d", i)
		if err := db.SaveDesiredService(ctx, DesiredService{Name: names[i], Image: "img:v1", Port: 8080}); err != nil {
			t.Fatalf("seed app %d: %v", i, err)
		}
	}
	base := time.Now().UTC().Add(-48 * time.Hour)
	for i := 0; i < scaleAttempts; i++ {
		a := DeployAttempt{
			ID: fmt.Sprintf("dep_%05d", i), ServiceName: names[i%scaleApps], Image: "img:v1",
			Source: DeployAttemptSourceManual, Status: DeployAttemptStatusSucceeded,
			StartedAt: base.Add(time.Duration(i) * time.Second),
		}
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatalf("seed attempt %d: %v", i, err)
		}
	}
	conds := []reconcile.Condition{{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Running"}}
	for _, n := range names {
		if err := db.UpsertConditions(ctx, "application/"+n, conds); err != nil {
			t.Fatalf("seed conditions: %v", err)
		}
	}
	t.Logf("seed %d apps, %d attempts, %d condition sets: %s", scaleApps, scaleAttempts, scaleApps, time.Since(seedStart).Round(time.Millisecond))

	report := func(label string, d []time.Duration) {
		p50, p95, p99 := scalePercentiles(d)
		t.Logf("%-46s p50 %7.2f ms  p95 %7.2f ms  p99 %7.2f ms", label, p50, p95, p99)
	}

	report(fmt.Sprintf("list apps, page of %d", scalePage), scaleTime(scaleSamples, func(i int) {
		if _, _, err := db.ListDesiredServicesFiltered(ctx, AppListFilter{Limit: scalePage, Offset: (i % 20) * scalePage}); err != nil {
			t.Fatal(err)
		}
	}))
	report("list apps, all 1000 (no limit)", scaleTime(scaleSamples/4, func(int) {
		if _, _, err := db.ListDesiredServicesFiltered(ctx, AppListFilter{}); err != nil {
			t.Fatal(err)
		}
	}))
	report("list apps, search q=app-05", scaleTime(scaleSamples, func(int) {
		if _, _, err := db.ListDesiredServicesFiltered(ctx, AppListFilter{Query: "app-05", Limit: scalePage}); err != nil {
			t.Fatal(err)
		}
	}))
	page := make([]string, scalePage)
	for i := range page {
		page[i] = "application/" + names[i]
	}
	report(fmt.Sprintf("conditions batch, %d controllers", scalePage), scaleTime(scaleSamples, func(int) {
		if _, err := db.GetConditionsForControllers(ctx, page); err != nil {
			t.Fatal(err)
		}
	}))
	report("latest deploy activity, all apps", scaleTime(scaleSamples/4, func(int) {
		if _, err := db.LatestDeployActivity(ctx); err != nil {
			t.Fatal(err)
		}
	}))
	report("deploy attempts for one app (5 rows)", scaleTime(scaleSamples, func(i int) {
		if _, err := db.ListDeployAttempts(ctx, names[i%scaleApps]); err != nil {
			t.Fatal(err)
		}
	}))
	sweep := scaleTime(5, func(int) {
		for _, n := range names {
			if err := db.UpsertConditions(ctx, "application/"+n, conds); err != nil {
				t.Fatal(err)
			}
		}
	})
	report("reconcile status write sweep, 1000 controllers", sweep)

	m := db.Metrics()
	t.Logf("db: %d queries, %d slow (over %d ms), p95 %.2f ms, wal %d KiB, file %d KiB", m.Queries, m.SlowQueries, m.SlowThresholdMS, m.P95MS, m.WALBytes>>10, m.DBBytes>>10)
}

func TestDBMetricsSlowQueryAndCheckpoint(t *testing.T) {
	t.Setenv(envSlowQueryMS, "1")
	db := openTestDB(t)
	ctx := context.Background()
	db.m = newDBMetrics(db.m.path)
	if _, err := db.ExecContext(ctx, "WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x < 300000) SELECT count(*) FROM c"); err != nil {
		t.Fatal(err)
	}
	m := db.Metrics()
	if m.Queries == 0 || m.SlowQueries == 0 || len(m.RecentSlow) == 0 {
		t.Fatalf("slow query not recorded: %+v", m)
	}
	db.ResetSlowQueries()
	if got := db.Metrics(); got.SlowQueries != 0 || len(got.RecentSlow) != 0 {
		t.Fatalf("reset did not clear: %+v", got)
	}
	if err := db.CheckpointWAL(ctx, true); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if err := db.Optimize(ctx); err != nil {
		t.Fatalf("optimize: %v", err)
	}
}
