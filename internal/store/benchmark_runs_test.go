package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func benchRun(n int) BenchmarkRun {
	return BenchmarkRun{
		BenchmarkRunSummary: BenchmarkRunSummary{
			ID: fmt.Sprintf("bench_%02d", n), GeneratedAt: time.Unix(1700000000+int64(n)*60, 0), Version: "v1", Commit: "abc",
			HostSummary: "linux/arm64, 4 CPUs", HostOS: "linux", Repeat: 1, Passed: 2, Failed: 1, RecordedBy: "tester",
		},
		Report: []byte(`{"schema_version":1}`),
	}
}

func TestBenchmarkRunsRoundTripPruneAndDelete(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for i := 1; i <= 4; i++ {
		if _, created, err := db.RecordBenchmarkRun(ctx, benchRun(i), 3); err != nil || !created {
			t.Fatalf("record %d: created=%v err=%v", i, created, err)
		}
	}
	list, err := db.ListBenchmarkRuns(ctx, 10)
	if err != nil || len(list) != 3 {
		t.Fatalf("list = %d rows, err %v, want 3 after pruning to 3", len(list), err)
	}
	if list[0].ID != "bench_04" || list[2].ID != "bench_02" {
		t.Fatalf("order/prune wrong: %s .. %s", list[0].ID, list[2].ID)
	}

	if _, created, err := db.RecordBenchmarkRun(ctx, benchRun(4), 3); err != nil || created {
		t.Fatalf("same file twice: created=%v err=%v", created, err)
	}

	got, err := db.GetBenchmarkRun(ctx, "bench_03")
	if err != nil || string(got.Report) != `{"schema_version":1}` || got.Failed != 1 || got.RecordedBy != "tester" {
		t.Fatalf("get = %+v err=%v", got, err)
	}
	if _, err := db.GetBenchmarkRun(ctx, "missing"); !errors.Is(err, ErrBenchmarkRunNotFound) {
		t.Fatalf("missing get err = %v", err)
	}

	if err := db.DeleteBenchmarkRun(ctx, "bench_03"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteBenchmarkRun(ctx, "bench_03"); !errors.Is(err, ErrBenchmarkRunNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
}
