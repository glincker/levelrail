package telemetry

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// A reader on the single telemetry connection must get in between sweep batches.
func TestPrune_ConcurrentReaderNotStarved(t *testing.T) {
	ctx := context.Background()
	start := time.Unix(1_699_999_200, 0).UTC()
	const apps, metrics, ticks = 100, 7, 480
	cfg := testTierConfig()
	cfg.RawRetention = time.Hour
	db, _ := newClockDB(t, cfg, start.Add(2*time.Hour))
	for k := 0; k < ticks; k++ {
		rows := make([]Sample, 0, apps*metrics)
		ts := start.Add(time.Duration(k) * 15 * time.Second)
		for a := 0; a < apps; a++ {
			for m := 0; m < metrics; m++ {
				rows = append(rows, Sample{ResourceID: fmt.Sprintf("app:%d", a), Metric: fmt.Sprintf("m%d", m), Timestamp: ts, Value: 1})
			}
		}
		if err := db.WriteSamples(ctx, rows); err != nil {
			t.Fatal(err)
		}
	}
	SetRetentionTuningSource(func() RetentionTuning {
		return RetentionTuning{BatchRows: 2000, SweepBudget: time.Minute, Pause: 5 * time.Millisecond}
	})
	t.Cleanup(func() {
		SetRetentionTuningSource(func() RetentionTuning {
			return RetentionTuning{BatchRows: defaultRetentionBatchRows, SweepBudget: defaultRetentionBudget, Pause: defaultRetentionPause}
		})
	})

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
			_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM metric_samples WHERE resource_id = ? AND metric = ?`, "app:0", "m0").Scan(&n)
			maxRead = max(maxRead, time.Since(s))
			time.Sleep(time.Millisecond)
		}
	}()
	s := time.Now()
	res, err := db.Prune(ctx, start.Add(2*time.Hour))
	took := time.Since(s)
	close(stop)
	wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("deleted=%d took=%s max_read=%s", res.Raw, took, maxRead)
	if res.Raw == 0 {
		t.Fatal("nothing pruned")
	}
	if maxRead > took/4 {
		t.Errorf("reader waited %s during a %s sweep", maxRead, took)
	}
}
