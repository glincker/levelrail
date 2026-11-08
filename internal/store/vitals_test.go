package store

import (
	"context"
	"testing"
	"time"
)

func TestVitalsDeleteDesiredServiceClearsRows(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "shop", Image: "nginx"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetVitalsEnabled(ctx, "shop", true); err != nil {
		t.Fatal(err)
	}
	rows := []VitalsCount{{App: "shop", Route: "/", Metric: "LCP", Device: "desktop", Hour: time.Now(), Bucket: 2, Count: 3}}
	if err := db.AddVitalsCounts(ctx, rows); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteDesiredService(ctx, "shop"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := db.GetVitalsConfig(ctx, "shop"); found {
		t.Fatal("vitals config survived app delete")
	}
	got, err := db.QueryVitals(ctx, "shop", time.Now().Add(-time.Hour), 10)
	if err != nil || len(got) != 0 {
		t.Fatalf("rollups after delete = %v, %v", got, err)
	}
}

func TestVitalsCountsTopRoutesAndBatchPrune(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	old := now.Add(-48 * time.Hour)
	rows := []VitalsCount{
		{App: "a", Route: "/big", Metric: "LCP", Device: "desktop", Hour: now, Bucket: 1, Count: 9},
		{App: "a", Route: "/big", Metric: "LCP", Device: "desktop", Hour: now, Bucket: 1, Count: 1},
		{App: "a", Route: "/mid", Metric: "LCP", Device: "desktop", Hour: now, Bucket: 1, Count: 5},
		{App: "a", Route: "/small", Metric: "LCP", Device: "desktop", Hour: now, Bucket: 1, Count: 1},
	}
	for i := range 5 {
		rows = append(rows, VitalsCount{App: "a", Route: "/old", Metric: "LCP", Device: "desktop", Hour: old.Add(-time.Duration(i) * time.Hour), Bucket: 1, Count: 1})
	}
	if err := db.AddVitalsCounts(ctx, rows); err != nil {
		t.Fatal(err)
	}
	got, err := db.QueryVitals(ctx, "a", now.Add(-time.Hour), 2)
	if err != nil || len(got) != 2 || got[0].Count+got[1].Count != 15 {
		t.Fatalf("top 2 = %+v, %v", got, err)
	}
	var total int64
	for {
		n, err := db.PruneVitalsBatch(ctx, now.Add(-24*time.Hour), 2)
		if err != nil {
			t.Fatal(err)
		}
		total += n
		if n < 2 {
			break
		}
	}
	if total != 5 {
		t.Fatalf("pruned %d, want 5", total)
	}
	if left, _ := db.VitalsRoutesSince(ctx, "a", old.Add(-24*time.Hour), 10); len(left) != 3 {
		t.Fatalf("routes left = %v", left)
	}
}

func TestQueryVitalsOverallIgnoresRouteLimit(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	rows := []VitalsCount{
		{App: "a", Route: "/big", Metric: "LCP", Device: "desktop", Hour: now, Bucket: 1, Count: 9},
		{App: "a", Route: "/quiet", Metric: "LCP", Device: "desktop", Hour: now, Bucket: 5, Count: 2},
	}
	if err := db.AddVitalsCounts(ctx, rows); err != nil {
		t.Fatal(err)
	}
	got, err := db.QueryVitalsOverall(ctx, "a", now.Add(-time.Hour))
	if err != nil || len(got) != 2 {
		t.Fatalf("overall = %+v, %v", got, err)
	}
}
