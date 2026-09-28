package telemetry

import (
	"context"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newClockDB(t *testing.T, cfg TierConfig, start time.Time) (*DB, *fakeClock) {
	t.Helper()
	db := newTestDB(t)
	clk := &fakeClock{t: start}
	db.SetClock(clk.now)
	db.Configure(cfg)
	return db, clk
}

func testTierConfig() TierConfig {
	cfg := DefaultTierConfig()
	cfg.Lag = 0
	cfg.MaxBytes = 0
	return cfg
}

// seed writes cpu (gauge, value 10 then 20 alternating) and http_requests
// (counter, 1 per tick) every 15s over [start, start+span).
func seed(t *testing.T, db *DB, start time.Time, span time.Duration) {
	t.Helper()
	var rows []Sample
	i := 0
	for ts := start; ts.Before(start.Add(span)); ts = ts.Add(15 * time.Second) {
		cpu := 10.0
		if i%2 == 1 {
			cpu = 20
		}
		rows = append(rows,
			Sample{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: ts, Value: cpu},
			Sample{ResourceID: "service:web", Metric: MetricHTTPRequests, Timestamp: ts, Value: 1},
		)
		i++
	}
	if err := db.WriteSamples(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
}

func TestRollup_MinuteAndHourValues(t *testing.T) {
	ctx := context.Background()
	start := time.Unix(1_699_999_200, 0).UTC()
	db, clk := newClockDB(t, testTierConfig(), start.Add(3*time.Hour+time.Minute))
	seed(t, db, start, 3*time.Hour)

	res, err := db.Rollup(ctx, clk.now())
	if err != nil {
		t.Fatal(err)
	}
	if res.More || res.Buckets == 0 {
		t.Fatalf("Rollup() = %+v", res)
	}

	tests := []struct {
		name   string
		tier   int
		metric string
		want   float64
	}{
		{"minute gauge averages", tierMinute, "cpu_percent", 15},
		{"minute counter sums", tierMinute, MetricHTTPRequests, 4},
		{"hour gauge averages", tierHour, "cpu_percent", 15},
		{"hour counter sums", tierHour, MetricHTTPRequests, 240},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := db.queryRollup(ctx, tc.tier, "service:web", tc.metric, start, start.Add(3*time.Hour))
			if err != nil || len(got) == 0 {
				t.Fatalf("queryRollup() = %v, %v", got, err)
			}
			if got[0].Value != tc.want {
				t.Errorf("first bucket = %v, want %v", got[0].Value, tc.want)
			}
		})
	}
}

func TestRollup_ResumesInBoundedChunks(t *testing.T) {
	ctx := context.Background()
	start := time.Unix(1_699_999_200, 0).UTC()
	cfg := testTierConfig()
	cfg.MaxBucketsPerRun = 30
	db, clk := newClockDB(t, cfg, start.Add(2*time.Hour))
	seed(t, db, start, 2*time.Hour)

	var marks []int64
	for i := 0; i < 20; i++ {
		res, err := db.Rollup(ctx, clk.now())
		if err != nil {
			t.Fatal(err)
		}
		done, _ := db.rollupWatermark(ctx, tierMinute)
		marks = append(marks, done)
		if !res.More {
			break
		}
	}
	if len(marks) != 4 {
		t.Fatalf("runs = %d (%v), want 4 chunks of 30 minutes over 2h", len(marks), marks)
	}
	for i := 1; i < len(marks); i++ {
		if marks[i]-marks[i-1] > 30*60 || marks[i] <= marks[i-1] {
			t.Errorf("watermark step %d = %d, want bounded forward progress", i, marks[i]-marks[i-1])
		}
	}

	whole, _ := newClockDB(t, testTierConfig(), start.Add(2*time.Hour))
	seed(t, whole, start, 2*time.Hour)
	if _, err := whole.Rollup(ctx, clk.now()); err != nil {
		t.Fatal(err)
	}
	a, _ := db.queryRollup(ctx, tierMinute, "service:web", MetricHTTPRequests, start, start.Add(3*time.Hour))
	b, _ := whole.queryRollup(ctx, tierMinute, "service:web", MetricHTTPRequests, start, start.Add(3*time.Hour))
	if len(a) != len(b) || len(a) == 0 {
		t.Fatalf("chunked %d buckets vs single-run %d", len(a), len(b))
	}
}

func TestRollup_HalfFinishedRunRecomputes(t *testing.T) {
	ctx := context.Background()
	start := time.Unix(1_699_999_200, 0).UTC()
	db, clk := newClockDB(t, testTierConfig(), start.Add(10*time.Minute))
	seed(t, db, start, 5*time.Minute)

	// A crash between aggregate and watermark commit cannot persist (one
	// transaction), but a stale or partial bucket row must still be replaced.
	if _, err := db.ExecContext(ctx, `INSERT INTO metric_rollups (tier, resource_id, metric, ts, sum, min, max, count)
		VALUES (60, 'service:web', 'http_requests', ?, 999, 999, 999, 1)`, start.Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Rollup(ctx, clk.now()); err != nil {
		t.Fatal(err)
	}
	got, _ := db.queryRollup(ctx, tierMinute, "service:web", MetricHTTPRequests, start, start)
	if len(got) != 1 || got[0].Value != 4 {
		t.Errorf("recomputed bucket = %v, want sum 4", got)
	}
}

func TestRollup_CancelledRunLeavesWatermarkAndResumes(t *testing.T) {
	start := time.Unix(1_699_999_200, 0).UTC()
	db, clk := newClockDB(t, testTierConfig(), start.Add(10*time.Minute))
	seed(t, db, start, 5*time.Minute)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := db.Rollup(cancelled, clk.now()); err == nil {
		t.Fatal("Rollup with cancelled context must error")
	}
	if done, _ := db.rollupWatermark(context.Background(), tierMinute); done != 0 {
		t.Fatalf("watermark = %d after failed run, want 0", done)
	}
	if _, err := db.Rollup(context.Background(), clk.now()); err != nil {
		t.Fatal(err)
	}
	if done, _ := db.rollupWatermark(context.Background(), tierMinute); done == 0 {
		t.Error("watermark did not advance on resume")
	}
}

func TestRollup_OpenBucketsAreNotClosed(t *testing.T) {
	ctx := context.Background()
	start := time.Unix(1_699_999_200, 0).UTC()
	cfg := testTierConfig()
	cfg.Lag = 30 * time.Second
	db, clk := newClockDB(t, cfg, start.Add(90*time.Second))
	seed(t, db, start, 90*time.Second)
	if _, err := db.Rollup(ctx, clk.now()); err != nil {
		t.Fatal(err)
	}
	got, _ := db.queryRollup(ctx, tierMinute, "service:web", MetricHTTPRequests, start, start.Add(time.Hour))
	if len(got) != 1 {
		t.Errorf("closed minute buckets = %d, want 1 (second minute still inside lag)", len(got))
	}
}

func TestPickTier(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	db, _ := newClockDB(t, DefaultTierConfig(), now)
	tests := []struct {
		name string
		from time.Time
		want int
	}{
		{"last hour is raw", now.Add(-time.Hour), 0},
		{"exactly raw max range", now.Add(-6 * time.Hour), 0},
		{"one day is minute", now.Add(-24 * time.Hour), tierMinute},
		{"two weeks is hour", now.Add(-14 * 24 * time.Hour), tierHour},
		{"short range past raw retention falls to minute", now.Add(-20 * 24 * time.Hour), tierMinute},
		{"short range past minute retention falls to hour", now.Add(-40 * 24 * time.Hour), tierHour},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			to := tc.from.Add(time.Hour)
			if tc.name == "exactly raw max range" || tc.name == "one day is minute" || tc.name == "two weeks is hour" {
				to = now
			}
			if got := db.pickTier(tc.from, to); got != tc.want {
				t.Errorf("pickTier() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestQuery_UsesRollupAndTopsUpWithFinerTail(t *testing.T) {
	ctx := context.Background()
	start := time.Unix(1_699_999_200, 0).UTC()
	end := start.Add(12 * time.Hour)
	cfg := testTierConfig()
	cfg.Lag = time.Minute
	db, clk := newClockDB(t, cfg, end)
	seed(t, db, start, 12*time.Hour)
	if _, err := db.Rollup(ctx, clk.now()); err != nil {
		t.Fatal(err)
	}

	got, err := db.Query(ctx, "service:web", MetricHTTPRequests, start, end)
	if err != nil {
		t.Fatal(err)
	}
	var total float64
	for _, s := range got {
		total += s.Value
	}
	if want := float64(12 * 3600 / 15); total != want {
		t.Errorf("summed requests = %v, want %v (no double count at the rollup/raw seam)", total, want)
	}
	if len(got) >= 12*3600/15 {
		t.Errorf("points = %d, want rollup-sized result", len(got))
	}
}

func TestQuery_FallsBackToRawBeforeFirstRollup(t *testing.T) {
	ctx := context.Background()
	start := time.Unix(1_699_999_200, 0).UTC()
	db, _ := newClockDB(t, testTierConfig(), start.Add(12*time.Hour))
	seed(t, db, start.Add(11*time.Hour), time.Hour)
	got, err := db.Query(ctx, "service:web", "cpu_percent", start, start.Add(12*time.Hour))
	if err != nil || len(got) != 240 {
		t.Errorf("Query() = %d samples, %v; want 240 raw samples", len(got), err)
	}
}

func TestPrune_AgeAndUnrolledRawProtectedBySizeCap(t *testing.T) {
	ctx := context.Background()
	start := time.Unix(1_699_999_200, 0).UTC()
	cfg := testTierConfig()
	cfg.RawRetention = 2 * time.Hour
	db, clk := newClockDB(t, cfg, start.Add(3*time.Hour))
	seed(t, db, start, 3*time.Hour)

	res, err := db.Prune(ctx, clk.now())
	if err != nil {
		t.Fatal(err)
	}
	if res.Raw == 0 {
		t.Fatal("age prune removed nothing")
	}
	var oldest int64
	_ = db.QueryRowContext(ctx, `SELECT MIN(ts) FROM metric_samples`).Scan(&oldest)
	if oldest < clk.now().Add(-2*time.Hour).Unix() {
		t.Errorf("oldest raw ts %d is older than retention", oldest)
	}

	tiny := cfg
	tiny.RawRetention = 24 * time.Hour
	tiny.MaxBytes = 1
	db.Configure(tiny)
	var before int64
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM metric_samples`).Scan(&before)
	res, err = db.Prune(ctx, clk.now())
	if err != nil {
		t.Fatal(err)
	}
	var after int64
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM metric_samples`).Scan(&after)
	if before != after || !res.OverCap {
		t.Errorf("unrolled raw rows must survive the size cap: before %d after %d overcap %v", before, after, res.OverCap)
	}
}

func TestPrune_SizeCapDropsRolledUpRawFirst(t *testing.T) {
	ctx := context.Background()
	start := time.Unix(1_699_999_200, 0).UTC()
	cfg := testTierConfig()
	db, clk := newClockDB(t, cfg, start.Add(6*time.Hour))
	seed(t, db, start, 6*time.Hour)
	if _, err := db.Rollup(ctx, clk.now()); err != nil {
		t.Fatal(err)
	}
	size, err := db.liveBytes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	capped := cfg
	capped.MaxBytes = size * 6 / 10
	db.Configure(capped)

	res, err := db.Prune(ctx, clk.now())
	if err != nil {
		t.Fatal(err)
	}
	if res.Raw == 0 {
		t.Fatalf("size cap pruned no raw rows: %+v", res)
	}
	var minuteRows int64
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM metric_rollups WHERE tier = 60`).Scan(&minuteRows)
	if minuteRows == 0 {
		t.Error("minute rollups must outlive raw rows under the size cap")
	}
	done, _ := db.rollupWatermark(ctx, tierMinute)
	var newestPruned int64
	_ = db.QueryRowContext(ctx, `SELECT MIN(ts) FROM metric_samples`).Scan(&newestPruned)
	if newestPruned > done {
		t.Errorf("raw pruned past the rollup watermark: min raw ts %d > %d", newestPruned, done)
	}
}

func TestTierConfigFromEnv(t *testing.T) {
	env := map[string]string{
		"APP_METRICS_RETENTION":              "48h",
		"APP_METRICS_MAX_DB_BYTES":           "5000",
		"APP_METRICS_RAW_QUERY_MAX_RANGE":    "nonsense",
		"APP_METRICS_ROLLUP_MAX_BUCKETS":     "10",
		"APP_METRICS_MINUTE_QUERY_MAX_RANGE": "-5h",
	}
	cfg := TierConfigFromEnv(func(k string) string { return env[k] })
	def := DefaultTierConfig()
	if cfg.RawRetention != 48*time.Hour || cfg.MaxBytes != 5000 || cfg.MaxBucketsPerRun != 10 {
		t.Errorf("overrides not applied: %+v", cfg)
	}
	if cfg.RawMaxRange != def.RawMaxRange || cfg.MinuteMaxRange != def.MinuteMaxRange {
		t.Errorf("invalid values must fall back to defaults: %+v", cfg)
	}
}
