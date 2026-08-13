package telemetry

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "telemetry.db")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestWriteSamples_EmptyIsNoop(t *testing.T) {
	db := newTestDB(t)
	if err := db.WriteSamples(context.Background(), nil); err != nil {
		t.Errorf("WriteSamples(nil) error = %v, want nil", err)
	}
}

func TestWriteAndQuery_RoundTrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	base := time.Unix(1_700_000_000, 0).UTC()
	samples := []Sample{
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: base, Value: 12.5},
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: base.Add(15 * time.Second), Value: 20.0},
		{ResourceID: "service:web", Metric: "memory_bytes", Timestamp: base, Value: 1024},
		{ResourceID: "service:worker", Metric: "cpu_percent", Timestamp: base, Value: 5.0}, // different resource: must not leak into service:web's results
	}
	if err := db.WriteSamples(ctx, samples); err != nil {
		t.Fatalf("WriteSamples() error = %v", err)
	}

	got, err := db.Query(ctx, "service:web", "cpu_percent", base.Add(-time.Hour), base.Add(time.Hour))
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Query() returned %d samples, want 2", len(got))
	}
	if got[0].Value != 12.5 || got[1].Value != 20.0 {
		t.Errorf("Query() values = [%v, %v], want [12.5, 20.0] in timestamp order", got[0].Value, got[1].Value)
	}
	if !got[0].Timestamp.Equal(base) {
		t.Errorf("Query()[0].Timestamp = %v, want %v", got[0].Timestamp, base)
	}
}

func TestQuery_NoSamples_ReturnsEmptyNotError(t *testing.T) {
	db := newTestDB(t)
	got, err := db.Query(context.Background(), "service:nonexistent", "cpu_percent", time.Unix(0, 0), time.Now())
	if err != nil {
		t.Fatalf("Query() error = %v, want nil (no samples is a valid observed state)", err)
	}
	if len(got) != 0 {
		t.Errorf("Query() = %d samples, want 0", len(got))
	}
}

func TestQuery_RespectsTimeRange(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	base := time.Unix(1_700_000_000, 0).UTC()
	err := db.WriteSamples(ctx, []Sample{
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: base.Add(-time.Hour), Value: 1},
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: base, Value: 2},
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: base.Add(time.Hour), Value: 3},
	})
	if err != nil {
		t.Fatalf("WriteSamples() error = %v", err)
	}

	got, err := db.Query(ctx, "service:web", "cpu_percent", base.Add(-time.Minute), base.Add(time.Minute))
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(got) != 1 || got[0].Value != 2 {
		t.Errorf("Query() with a tight range = %+v, want exactly the one sample inside [from, to]", got)
	}
}

func TestWriteSamples_SameKeyTwice_ReplacesValue(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	ts := time.Unix(1_700_000_000, 0).UTC()

	if err := db.WriteSamples(ctx, []Sample{{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: ts, Value: 1}}); err != nil {
		t.Fatalf("first WriteSamples() error = %v", err)
	}
	if err := db.WriteSamples(ctx, []Sample{{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: ts, Value: 2}}); err != nil {
		t.Fatalf("second WriteSamples() error = %v", err)
	}

	got, err := db.Query(ctx, "service:web", "cpu_percent", ts.Add(-time.Second), ts.Add(time.Second))
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Query() = %d rows, want exactly 1 (same key replaces, not duplicates)", len(got))
	}
	if got[0].Value != 2 {
		t.Errorf("Query()[0].Value = %v, want 2 (the second write's value)", got[0].Value)
	}
}

func TestRetain_DeletesOnlyOlderThanCutoff(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	err := db.WriteSamples(ctx, []Sample{
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: base.Add(-48 * time.Hour), Value: 1}, // old, should go
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: base.Add(-1 * time.Hour), Value: 2},  // recent, should stay
	})
	if err != nil {
		t.Fatalf("WriteSamples() error = %v", err)
	}

	deleted, err := db.Retain(ctx, base.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("Retain() error = %v", err)
	}
	if deleted != 1 {
		t.Errorf("Retain() deleted = %d, want 1", deleted)
	}

	remaining, err := db.Query(ctx, "service:web", "cpu_percent", base.Add(-72*time.Hour), base)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(remaining) != 1 || remaining[0].Value != 2 {
		t.Errorf("remaining samples after Retain() = %+v, want exactly the recent one", remaining)
	}
}
