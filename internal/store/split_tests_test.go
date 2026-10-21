package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/splittest"
)

func sampleSplit(t *testing.T, app string) SplitTest {
	t.Helper()
	id, err := NewSplitTestID()
	if err != nil {
		t.Fatal(err)
	}
	return SplitTest{
		ID: id, AppName: app, Status: splittest.StatusRunning, CookieName: "lr_split", CookieTTLSecs: 3600,
		Variants: []splittest.Variant{
			{ID: "a", Label: "Control", Kind: splittest.KindProduction, Weight: 50},
			{ID: "b", Label: "New", Kind: splittest.KindBranch, Ref: "new", Weight: 50},
		},
	}
}

func TestSplitTestLifecycle(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	first := sampleSplit(t, "web")
	if err := db.CreateSplitTest(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateSplitTest(ctx, sampleSplit(t, "web")); !errors.Is(err, ErrSplitTestRunning) {
		t.Fatalf("second running test err = %v, want ErrSplitTestRunning", err)
	}
	if err := db.CreateSplitTest(ctx, sampleSplit(t, "other")); err != nil {
		t.Fatalf("other app must be independent: %v", err)
	}

	got, ok, err := db.GetRunningSplitTest(ctx, "web")
	if err != nil || !ok || len(got.Variants) != 2 || got.CookieName != "lr_split" {
		t.Fatalf("running test = %+v ok=%v err=%v", got, ok, err)
	}

	got.Variants[0].Weight, got.Variants[1].Weight = 80, 20
	if err := db.UpdateSplitTestVariants(ctx, got.ID, got.Variants); err != nil {
		t.Fatal(err)
	}
	got, _, _ = db.GetRunningSplitTest(ctx, "web")
	if got.Variants[1].Weight != 20 {
		t.Fatalf("weights not saved: %+v", got.Variants)
	}

	if all, _ := db.ListRunningSplitTests(ctx); len(all) != 2 {
		t.Fatalf("running tests = %d, want 2", len(all))
	}
	if err := db.EndSplitTest(ctx, got.ID, "stopped"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.GetRunningSplitTest(ctx, "web"); ok {
		t.Fatal("ended test must not be running")
	}
	latest, ok, _ := db.GetLatestSplitTest(ctx, "web")
	if !ok || latest.Status != splittest.StatusStopped || latest.EndReason != "stopped" || latest.EndedAt.IsZero() {
		t.Fatalf("latest = %+v ok=%v", latest, ok)
	}
	if err := db.CreateSplitTest(ctx, sampleSplit(t, "web")); err != nil {
		t.Fatalf("a new test after stopping must be allowed: %v", err)
	}
}

func TestSplitStatsAccumulatePerHour(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	at := time.Date(2026, 10, 20, 10, 30, 0, 0, time.UTC)
	if err := db.AddSplitStats(ctx, "t", map[string]uint64{"a": 5, "b": 2}, at); err != nil {
		t.Fatal(err)
	}
	if err := db.AddSplitStats(ctx, "t", map[string]uint64{"a": 3}, at.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.AddSplitStats(ctx, "t", map[string]uint64{"a": 1}, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListSplitStats(ctx, "t")
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows = %+v err=%v", rows, err)
	}
	if rows[0].VariantID != "a" || rows[0].Requests != 8 || rows[1].VariantID != "b" || rows[2].Requests != 1 {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}
