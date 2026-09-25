package telemetry

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestPercentileFromBuckets(t *testing.T) {
	tests := []struct {
		name   string
		counts []float64
		q      float64
		want   float64
	}{
		{"empty", make([]float64, LatencyBucketCount), 0.95, 0},
		{"all in first bucket median", bucketsWith(map[int]float64{0: 10}), 0.5, 2.5},
		{"split p50 lands on boundary", bucketsWith(map[int]float64{0: 50, 3: 50}), 0.5, 5},
		{"overflow reports last bound", bucketsWith(map[int]float64{11: 5}), 0.99, 10000},
		{"p95 in slow tail", bucketsWith(map[int]float64{1: 90, 5: 10}), 0.95, 175},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PercentileFromBuckets(tc.counts, tc.q)
			if math.Abs(got-tc.want) > 0.01 {
				t.Errorf("PercentileFromBuckets() = %v, want %v", got, tc.want)
			}
		})
	}
}

func bucketsWith(m map[int]float64) []float64 {
	out := make([]float64, LatencyBucketCount)
	for i, v := range m {
		out[i] = v
	}
	return out
}

func TestRequestWindowSamples_OnlyNonZero(t *testing.T) {
	w := RequestWindow{Requests: 3, Status2xx: 3, BytesOut: 10}
	w.Latency[2] = 3
	got := w.Samples("service:web", time.Unix(100, 0))
	names := map[string]float64{}
	for _, s := range got {
		names[s.Metric] = s.Value
	}
	if len(names) != 4 || names[MetricHTTPRequests] != 3 || names["http_latency_bucket_25"] != 3 {
		t.Errorf("Samples() = %v", names)
	}
}

func TestLatencyBucketMetric(t *testing.T) {
	if got := LatencyBucketMetric(0); got != "http_latency_bucket_5" {
		t.Errorf("bucket 0 = %q", got)
	}
	if got := LatencyBucketMetric(LatencyBucketCount - 1); got != "http_latency_bucket_inf" {
		t.Errorf("last bucket = %q", got)
	}
	if !IsSumMetric("http_requests") || IsSumMetric("cpu_percent") {
		t.Error("IsSumMetric misclassified")
	}
}

func TestQueryAndSummarizeRequests(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_010, 0).UTC()

	var w1, w2 RequestWindow
	w1.Requests, w1.Status2xx, w1.BytesOut = 100, 90, 1500
	w1.Status4xx, w1.Status5xx = 5, 5
	w1.Latency[1] = 100
	w2.Requests, w2.Status2xx = 100, 100
	w2.Latency[1] = 90
	w2.Latency[6] = 10
	if err := db.RecordRequests(ctx, map[string]RequestWindow{"web": w1}, base); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordRequests(ctx, map[string]RequestWindow{"web": w2}, base.Add(15*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordRequests(ctx, map[string]RequestWindow{"other": w1}, base); err != nil {
		t.Fatal(err)
	}
	fed := NewLocalFederator(db)

	points, err := QueryRequests(ctx, fed, "web", base, base.Add(time.Minute), 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 {
		t.Fatalf("points = %d, want 2", len(points))
	}
	if points[0].RatePerSec != 100.0/15 || points[0].ErrorRate5xx != 0.05 || points[0].ErrorRate4xx != 0.05 {
		t.Errorf("point0 = %+v", points[0])
	}
	if points[1].P95Ms <= points[0].P95Ms {
		t.Errorf("p95 should rise with the slow tail: %v vs %v", points[1].P95Ms, points[0].P95Ms)
	}

	sum, err := SummarizeRequests(ctx, fed, "web", time.Minute, base.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !sum.HasTraffic || sum.Requests != 200 || sum.ErrorRate5xx != 0.025 {
		t.Errorf("summary = %+v", sum)
	}

	empty, err := SummarizeRequests(ctx, fed, "ghost", time.Minute, base)
	if err != nil || empty.HasTraffic || empty.Requests != 0 {
		t.Errorf("empty summary = %+v, err %v", empty, err)
	}
	if _, err := SummarizeRequests(ctx, fed, "web", 0, base); err == nil {
		t.Error("zero window must error")
	}
}
