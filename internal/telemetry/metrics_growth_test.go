package telemetry

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

// TestMetricsGrowth200Apps measures raw sample storage for 200 apps over a
// simulated 24h at the live 15s tick. Skipped under -short; numbers go to t.Log.
func TestMetricsGrowth200Apps(t *testing.T) {
	if testing.Short() {
		t.Skip("growth measurement is slow")
	}
	const (
		apps     = 200
		interval = 15 * time.Second
		span     = 24 * time.Hour
	)
	ctx := context.Background()
	path := t.TempDir() + "/telemetry.db"
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	names := []string{"cpu_percent", "memory_usage_bytes", "memory_limit_bytes", "network_rx_bytes", "network_tx_bytes", "disk_read_bytes", "disk_write_bytes"}
	ids := make([]string, apps)
	for i := range ids {
		ids[i] = fmt.Sprintf("service:app-%03d", i)
	}

	ticks := int(span / interval)
	began := time.Now()
	var rows int64
	for k := 0; k < ticks; k++ {
		at := start.Add(time.Duration(k) * interval)
		batch := make([]Sample, 0, apps*len(names))
		for i, id := range ids {
			for m, name := range names {
				batch = append(batch, Sample{ResourceID: id, Metric: name, Timestamp: at, Value: float64((k*7+i*13+m*31)%100000) / 7})
			}
		}
		if err := db.WriteSamples(ctx, batch); err != nil {
			t.Fatal(err)
		}
		rows += int64(len(batch))
	}
	insertDur := time.Since(began)

	var count int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM metric_samples`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	_, _ = db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("inserted=%d rows=%d file_bytes=%d bytes_per_row=%.1f insert_total=%s per_tick=%s",
		rows, count, fi.Size(), float64(fi.Size())/float64(count), insertDur, insertDur/time.Duration(ticks))

	end := start.Add(span)
	db.SetClock(func() time.Time { return end })

	// Batched sweep of the oldest hour, with a reader probing the single connection.
	hourCfg := DefaultTierConfig()
	hourCfg.RawRetention = span - time.Hour
	db.tiers = &hourCfg
	var wg sync.WaitGroup
	stop := make(chan struct{})
	var maxRead time.Duration
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			s := time.Now()
			var n int
			_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM metric_samples WHERE resource_id = ? AND metric = ?`, ids[0], names[0]).Scan(&n)
			if d := time.Since(s); d > maxRead {
				maxRead = d
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	s := time.Now()
	res, err := db.Prune(ctx, end)
	batched := time.Since(s)
	close(stop)
	wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("batched_prune_1h deleted=%d took=%s max_concurrent_read=%s", res.Raw, batched, maxRead)

	// One unbatched statement over the next hour, for comparison.
	cutoff := end.Add(-span + 2*time.Hour).Unix()
	s = time.Now()
	n, err := db.deleteBefore(ctx, `DELETE FROM metric_samples WHERE ts < ?`, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("unbatched_delete_1h deleted=%d took=%s", n, time.Since(s))
}
