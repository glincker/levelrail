package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAppMetricsQueryableByName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SetAppMetricsTarget(ctx, AppMetricsTarget{App: "web", Port: 9100, Path: "/metrics"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour)
	if err := db.InsertAppMetricSamples(ctx, "web", []AppMetricSample{
		{Name: "jobs_total", Labels: `{q="a"}`, At: now, Value: 3},
		{Name: "jobs_total", Labels: `{q="b"}`, At: now, Value: 4},
		{Name: "open_fds", At: old, Value: 9},
	}, 100); err != nil {
		t.Fatal(err)
	}
	names, err := db.ListAppMetricNames(ctx, "web", now.Add(-time.Hour), 10)
	if err != nil || len(names) != 1 || names[0] != "jobs_total" {
		t.Fatalf("names %v err %v", names, err)
	}
	got, err := db.QueryAppMetricSamples(ctx, "web", "jobs_total", now.Add(-time.Minute), now.Add(time.Minute), 10)
	if err != nil || len(got) != 2 {
		t.Fatalf("samples %v err %v", got, err)
	}
	if n, err := db.PruneAppMetricSamples(ctx, now.Add(-24*time.Hour)); err != nil || n != 1 {
		t.Fatalf("pruned %d err %v", n, err)
	}
	if other, _ := db.QueryAppMetricSamples(ctx, "api", "jobs_total", now.Add(-time.Minute), now.Add(time.Minute), 10); len(other) != 0 {
		t.Fatalf("another app read web's samples: %v", other)
	}
}

func TestAppMetricsRowCapAndDeleteRace(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	one := []AppMetricSample{{Name: "x", At: time.Now(), Value: 1}}
	if err := db.InsertAppMetricSamples(ctx, "gone", one, 10); !errors.Is(err, ErrAppMetricsTargetNotFound) {
		t.Fatalf("insert without target = %v", err)
	}
	if err := db.SetAppMetricsTarget(ctx, AppMetricsTarget{App: "web", Port: 9100, Path: "/metrics"}); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertAppMetricSamples(ctx, "web", one, 1); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertAppMetricSamples(ctx, "web", one, 1); !errors.Is(err, ErrAppMetricsRowCap) {
		t.Fatalf("insert over cap = %v", err)
	}
}

func TestAppMetricsDroppedWithApp(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppMetricsTarget(ctx, AppMetricsTarget{App: "web", Port: 9100, Path: "/metrics"}); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertAppMetricSamples(ctx, "web", []AppMetricSample{{Name: "x", Labels: `{a="b"}`, At: time.Now(), Value: 1}}, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM desired_services WHERE name = 'web'`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"app_metrics_targets", "app_metric_series", "app_metric_samples"} {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s has %d rows after app delete (err %v)", table, n, err)
		}
	}
}
