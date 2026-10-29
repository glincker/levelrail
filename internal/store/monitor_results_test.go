package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/monitor"
)

func TestMonitorResultsRollupAndPruneBatches(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	m := monitor.Monitor{ID: "mon_t", Name: "t", Kind: monitor.KindTCP, Interval: time.Minute, Timeout: time.Second, Enabled: true,
		Spec: monitor.Spec{Host: "example.com", Port: 443}, CreatedAt: time.Unix(1000, 0)}
	if err := db.SaveMonitor(ctx, m); err != nil {
		t.Fatal(err)
	}
	base := int64(7200)
	var batch []monitor.Result
	for i := 0; i < 10; i++ {
		batch = append(batch, monitor.Result{MonitorID: m.ID, At: base + int64(i), OK: i != 9, LatencyMS: 10 * (i + 1)})
	}
	if err := db.InsertResults(ctx, batch); err != nil {
		t.Fatal(err)
	}
	// A replayed batch must not double count the rollup.
	if err := db.InsertResults(ctx, batch[:3]); err != nil {
		t.Fatal(err)
	}
	roll, err := db.ListRollups(ctx, m.ID, base, 10)
	if err != nil || len(roll) != 1 {
		t.Fatalf("rollups = %v, err %v", roll, err)
	}
	if r := roll[0]; r.Total != 10 || r.Failed != 1 || r.LatSum != 450 || r.LatMax != 90 {
		t.Fatalf("rollup = %+v", r)
	}
	raw, err := db.ListResults(ctx, m.ID, base, 100)
	if err != nil || len(raw) != 10 || raw[0].At != base+9 {
		t.Fatalf("raw = %d rows, err %v", len(raw), err)
	}

	total := 0
	for rounds := 0; rounds < 10; rounds++ {
		n, err := db.PruneResults(ctx, m.ID, base+8, 3)
		if err != nil {
			t.Fatal(err)
		}
		if n > 3 {
			t.Fatalf("batch exceeded limit: %d", n)
		}
		total += n
		if n < 3 {
			break
		}
	}
	if total != 8 {
		t.Fatalf("pruned %d rows, want 8", total)
	}
	if n, _ := db.PruneRollups(ctx, m.ID, base+3600, 5); n != 1 {
		t.Fatalf("rollup prune = %d, want 1", n)
	}
	if err := db.DeleteMonitor(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteMonitor(ctx, m.ID); !errors.Is(err, monitor.ErrNotFound) {
		t.Fatalf("second delete = %v", err)
	}
}
