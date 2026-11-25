package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestAggregate_RollupCountAlgebra(t *testing.T) {
	from := time.Unix(1_700_000_000, 0).UTC()
	at := from.Add(5 * time.Second)
	tests := []struct {
		name                  string
		in                    []Sample
		wantVal, wantMin, max float64
		wantCount             int
	}{
		{
			name: "unequal counts weigh by count",
			in: []Sample{
				{Timestamp: at, Value: 10, Count: 100, Sum: 1000, Min: 2, Max: 40},
				{Timestamp: at.Add(time.Second), Value: 30, Count: 1, Sum: 30, Min: 30, Max: 30},
			},
			wantVal: 1030.0 / 101, wantMin: 2, max: 40, wantCount: 101,
		},
		{
			name: "rollup plus raw tail",
			in: []Sample{
				{Timestamp: at, Value: 10, Count: 4, Sum: 40, Min: 5, Max: 15},
				{Timestamp: at.Add(time.Second), Value: 100},
			},
			wantVal: 28, wantMin: 5, max: 100, wantCount: 5,
		},
		{
			name: "raw only",
			in: []Sample{
				{Timestamp: at, Value: 2}, {Timestamp: at.Add(time.Second), Value: 4},
			},
			wantVal: 3, wantMin: 2, max: 4, wantCount: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Aggregate(tc.in, from, time.Minute)
			if len(got) != 1 {
				t.Fatalf("points = %d, want 1", len(got))
			}
			p := got[0]
			if p.Value != tc.wantVal || p.Min != tc.wantMin || p.Max != tc.max || p.Count != tc.wantCount {
				t.Errorf("got %+v, want value %v min %v max %v count %d", p, tc.wantVal, tc.wantMin, tc.max, tc.wantCount)
			}
		})
	}
}

func TestAggregate_CumulativeUsesMaxOfMaxes(t *testing.T) {
	from := time.Unix(1_700_000_000, 0).UTC()
	at := from.Add(5 * time.Second)
	m := "network_rx_bytes"
	// sum/count here is 250, a wrong average; the running total is 900.
	in := []Sample{
		{Metric: m, Timestamp: at, Value: 900, Count: 2, Sum: 1000, Min: 100, Max: 900},
		{Metric: m, Timestamp: at.Add(time.Second), Value: 850, Count: 2, Sum: 0, Min: 0, Max: 850},
	}
	got := Aggregate(in, from, time.Minute)
	if len(got) != 1 || got[0].Value != 900 || got[0].Max != 900 || got[0].Count != 4 {
		t.Fatalf("got %+v, want value 900 (max of maxes), count 4", got)
	}
}

// A counter reset between buckets is not repaired: each bucket reports its own
// raw max and no increase is invented.
func TestQueryRollup_CumulativeReset(t *testing.T) {
	ctx := context.Background()
	start := time.Unix(1_699_999_200, 0).UTC()
	db, _ := newClockDB(t, testTierConfig(), start.Add(12*time.Hour))
	rows := []struct{ off, sum, min, max float64 }{
		{0, 5000, 1000, 2000},
		{60, 400, 50, 200},
	}
	for _, r := range rows {
		if _, err := db.ExecContext(ctx, `INSERT INTO metric_rollups (tier, resource_id, metric, ts, sum, min, max, count)
			VALUES (60, 'service:web', 'disk_read_bytes', ?, ?, ?, ?, 4)`, start.Unix()+int64(r.off), r.sum, r.min, r.max); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.queryRollup(ctx, 60, "service:web", "disk_read_bytes", start, start.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Value != 2000 || got[1].Value != 200 {
		t.Fatalf("got %+v, want raw maxes 2000 then 200", got)
	}
}

// A watermark that is not bucket aligned must not let raw points inside an
// partly rolled up bucket be counted from both tiers.
func TestQuery_SeamDoesNotDoubleCount(t *testing.T) {
	ctx := context.Background()
	start := time.Unix(1_699_999_200, 0).UTC()
	end := start.Add(12 * time.Hour)
	db, _ := newClockDB(t, testTierConfig(), end)
	seed(t, db, start, 12*time.Hour)

	// One rolled up minute bucket (4 raw ticks) with a watermark 30s into
	// the bucket, so raw ticks of the covered minute overlap it.
	if _, err := db.ExecContext(ctx, `INSERT INTO metric_rollups (tier, resource_id, metric, ts, sum, min, max, count)
		VALUES (60, 'service:web', 'cpu_percent', ?, 60, 10, 20, 4)`, start.Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO rollup_state (tier, done_through) VALUES (60, ?)`, start.Add(30*time.Second).Unix()); err != nil {
		t.Fatal(err)
	}
	got, err := db.queryTier(ctx, tierMinute, "service:web", "cpu_percent", start, start.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	agg := Aggregate(got, start, time.Minute)
	if len(agg) < 2 || agg[0].Count != 4 {
		t.Fatalf("first bucket = %+v, want count 4 from the rollup alone (points %d)", agg, len(agg))
	}
}
