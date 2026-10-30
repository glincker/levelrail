package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
)

func appOps(t *testing.T, db *DB, name string) []string {
	t.Helper()
	changes, err := db.ResourceChangesSince(context.Background(), ResourceKindApp, 0, 0)
	if err != nil {
		t.Fatalf("changes: %v", err)
	}
	var ops []string
	for _, c := range changes {
		if c.Name == name {
			ops = append(ops, c.Op)
		}
	}
	return ops
}

func TestAppDualWriteRecordsLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	svc := DesiredService{Name: "web", Image: "nginx:1", Port: 80, Env: map[string]string{"API_KEY": "hunter2"}}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	svc.Image = "nginx:2"
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateServiceSuspended(ctx, "web", true); err != nil {
		t.Fatal(err)
	}
	cond := []reconcile.Condition{{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Running", Message: "at 1"}}
	if err := db.UpsertConditions(ctx, "application/web", cond); err != nil {
		t.Fatal(err)
	}
	cond[0].Message = "at 2"
	if err := db.UpsertConditions(ctx, "application/web", cond); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprint(appOps(t, db, "web"))
	want := "[ADDED MODIFIED MODIFIED MODIFIED DELETED]"
	if got != want {
		t.Fatalf("ops = %s, want %s (a message-only condition change must not log)", got, want)
	}
	changes, _ := db.ResourceChangesSince(ctx, ResourceKindApp, 0, 0)
	for _, c := range changes {
		snap, err := AppSnapshot(c.Object)
		if err != nil {
			t.Fatal(err)
		}
		if v, ok := snap.Env["API_KEY"]; !ok || v != "" {
			t.Fatalf("snapshot at %d keeps env value %q", c.Version, v)
		}
	}
}

func TestBackfillAppResourceChanges(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for _, n := range []string{"a", "b"} {
		if err := db.SaveDesiredService(ctx, DesiredService{Name: n, Image: "nginx", Port: 80}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM resource_changes`); err != nil {
		t.Fatal(err)
	}
	n, err := db.BackfillAppResourceChanges(ctx)
	if err != nil || n != 2 {
		t.Fatalf("backfill = %d, %v; want 2", n, err)
	}
	if n, _ := db.BackfillAppResourceChanges(ctx); n != 0 {
		t.Fatalf("second backfill = %d, want 0", n)
	}
}

func openBenchDB(b *testing.B) *DB {
	b.Helper()
	db, err := Open(context.Background(), filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	b.Cleanup(func() { _ = db.Close() })
	return db
}

func BenchmarkSaveDesiredServiceWithChangeLog(b *testing.B) {
	db := openBenchDB(b)
	ctx := context.Background()
	for i := 0; i < 1000; i++ {
		_ = db.SaveDesiredService(ctx, DesiredService{Name: fmt.Sprintf("app-%d", i), Image: "nginx", Port: 80})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := db.SaveDesiredService(ctx, DesiredService{Name: fmt.Sprintf("app-%d", i%1000), Image: fmt.Sprintf("nginx:%d", i), Port: 80}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRecordAppChange(b *testing.B) {
	db := openBenchDB(b)
	ctx := context.Background()
	for i := 0; i < 1000; i++ {
		_ = db.SaveDesiredService(ctx, DesiredService{Name: fmt.Sprintf("app-%d", i), Image: "nginx", Port: 80})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := db.noteAppChange(ctx, fmt.Sprintf("app-%d", i%1000)); err != nil {
			b.Fatal(err)
		}
	}
}
