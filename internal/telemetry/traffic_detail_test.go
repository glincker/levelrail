package telemetry

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestObserveUpstreamCapsLabels(t *testing.T) {
	var w RequestWindow
	for i := 0; i < 10; i++ {
		w.ObserveUpstream(fmt.Sprintf("10.0.0.%d:80", i), 200, false, 10, 3)
	}
	w.ObserveUpstream("", 200, false, 10, 3)
	if len(w.Upstreams) != 4 {
		t.Fatalf("labels = %d, want 3 + overflow", len(w.Upstreams))
	}
	if got := w.Upstreams[OverflowUpstream].Requests; got != 7 {
		t.Errorf("overflow requests = %d, want 7", got)
	}
}

func TestObserveUpstreamCounts(t *testing.T) {
	var w RequestWindow
	w.ObserveUpstream("a:1", 200, false, 4, 5)
	w.ObserveUpstream("a:1", 404, false, 30, 5)
	w.ObserveUpstream("a:1", 502, true, 20000, 5)
	st := w.Upstreams["a:1"]
	if st.Requests != 3 || st.Errors4xx != 1 || st.Errors5xx != 1 || st.UpstreamErrors != 1 {
		t.Errorf("stat = %+v", st)
	}
	if st.Latency[0] != 1 || st.Latency[2] != 0 || st.Latency[3] != 1 || st.Latency[LatencyBucketCount-1] != 1 {
		t.Errorf("latency = %v", st.Latency)
	}
}

func TestAddSampleBounded(t *testing.T) {
	var w RequestWindow
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'a'
	}
	for i := 0; i < 10; i++ {
		w.AddSample(RequestSample{Route: string(long), RequestID: string(long)}, 4)
	}
	if len(w.Sampled) != 4 {
		t.Fatalf("samples = %d, want 4", len(w.Sampled))
	}
	if len(w.Sampled[0].Route) != maxSampleRoute || len(w.Sampled[0].RequestID) != maxSampleField {
		t.Errorf("fields not clipped: %d %d", len(w.Sampled[0].Route), len(w.Sampled[0].RequestID))
	}
}

func TestShouldSample(t *testing.T) {
	cfg := SampleConfig{Rate: 0.5, SlowMS: 1000, TickMax: 10}
	cases := []struct {
		name   string
		cfg    SampleConfig
		status int
		ms     float64
		upErr  bool
		want   bool
	}{
		{"fast ok", cfg, 200, 10, false, false},
		{"5xx always", SampleConfig{Rate: 0, SlowMS: 1000, TickMax: 10}, 503, 10, false, true},
		{"upstream error", cfg, 200, 10, true, true},
		{"slow at rate 1", SampleConfig{Rate: 1, SlowMS: 1000, TickMax: 10}, 200, 2000, false, true},
		{"slow at rate 0", SampleConfig{Rate: 0, SlowMS: 1000, TickMax: 10}, 200, 2000, false, false},
		{"disabled", SampleConfig{Rate: 1, SlowMS: 1, TickMax: 0}, 500, 2000, true, false},
	}
	for _, c := range cases {
		if got := c.cfg.ShouldSample(c.status, c.ms, c.upErr, "req"); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	kept := 0
	for i := 0; i < 2000; i++ {
		if cfg.ShouldSample(200, 5000, false, fmt.Sprintf("id-%d", i)) {
			kept++
		}
	}
	if kept < 800 || kept > 1200 {
		t.Errorf("rate 0.5 kept %d of 2000", kept)
	}
}

func TestTrafficDetailRoundTripAndRetention(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	var w RequestWindow
	w.Requests = 3
	w.ObserveUpstream("a:1", 200, false, 20, 5)
	w.ObserveUpstream("a:1", 500, false, 20, 5)
	w.AddSample(RequestSample{Time: now, Route: "/x", Method: "GET", Status: 500, DurationMS: 20, Upstream: "a:1", RequestID: "r1"}, 5)
	w.AddSample(RequestSample{Time: now.Add(-48 * time.Hour), Route: "/old", Method: "GET", Status: 500, DurationMS: 20}, 5)
	if err := db.RecordRequests(ctx, map[string]RequestWindow{"web": w}, now); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordRequests(ctx, map[string]RequestWindow{"web": w}, now); err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryUpstreams(ctx, "service:web", now.Add(-time.Hour), now.Add(time.Minute))
	if err != nil || len(rows) != 1 || rows[0].Requests != 4 || rows[0].Errors5xx != 2 {
		t.Fatalf("upstreams = %+v err=%v", rows, err)
	}
	if got := PercentileFromBuckets(rows[0].Latency, 0.5); got <= 10 || got > 25 {
		t.Errorf("p50 = %v", got)
	}
	if _, err := db.Prune(ctx, now); err != nil {
		t.Fatal(err)
	}
	samples, err := db.QueryRequestSamples(ctx, "service:web", now.Add(-72*time.Hour), now.Add(time.Minute), 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range samples {
		if s.Route == "/old" {
			t.Errorf("expired sample survived prune: %+v", s)
		}
	}
	if len(samples) != 2 {
		t.Errorf("samples = %d, want 2 recent", len(samples))
	}
}

func TestSampleRowCap(t *testing.T) {
	t.Setenv(EnvRequestSampleRows, "3")
	db := newTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	var w RequestWindow
	w.Requests = 8
	for i := 0; i < 8; i++ {
		w.AddSample(RequestSample{Time: now, Route: "/x", Status: 500}, 20)
	}
	if err := db.RecordRequests(ctx, map[string]RequestWindow{"web": w}, now); err != nil {
		t.Fatal(err)
	}
	got, err := db.QueryRequestSamples(ctx, "service:web", now.Add(-time.Hour), now.Add(time.Minute), 100)
	if err != nil || len(got) != 3 {
		t.Fatalf("rows = %d err=%v, want 3", len(got), err)
	}
}
