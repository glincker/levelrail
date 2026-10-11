package telemetry

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestLatestByMetric_PicksNewestPerResource(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	var samples []Sample
	for i := 0; i < 5; i++ {
		ts := now.Add(-time.Duration(i) * time.Minute)
		samples = append(samples,
			Sample{ResourceID: "service:a", Metric: "cpu_percent", Timestamp: ts, Value: float64(10 - i)},
			Sample{ResourceID: "service:b", Metric: "cpu_percent", Timestamp: ts, Value: float64(20 - i)},
			Sample{ResourceID: "service:b", Metric: "memory_usage_bytes", Timestamp: ts, Value: 99},
			Sample{ResourceID: "node:n1", Metric: "cpu_percent", Timestamp: ts.Add(time.Second), Value: 1},
		)
	}
	if err := db.WriteSamples(ctx, samples); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := db.LatestByMetric(ctx, "cpu_percent")
	if err != nil {
		t.Fatalf("LatestByMetric: %v", err)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].ResourceID < got[j].ResourceID })
	want := map[string]float64{"node:n1": 1, "service:a": 10, "service:b": 20}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(got), len(want), got)
	}
	for _, s := range got {
		if s.Value != want[s.ResourceID] || s.Metric != "cpu_percent" {
			t.Errorf("%s = %v (%s), want %v", s.ResourceID, s.Value, s.Metric, want[s.ResourceID])
		}
	}

	none, err := db.LatestByMetric(ctx, "absent_metric")
	if err != nil || len(none) != 0 {
		t.Errorf("absent metric = %v, %v, want empty", none, err)
	}
}

func TestLatestByMetric_EmptyTable(t *testing.T) {
	db := newTestDB(t)
	got, err := db.LatestByMetric(context.Background(), "cpu_percent")
	if err != nil || len(got) != 0 {
		t.Errorf("empty table = %v, %v", got, err)
	}
}

// RequestOverviews must equal the per app SummarizeRequests and
// QueryRequests it replaces, field for field.
func TestRequestOverviews_MatchesPerAppQueries(t *testing.T) {
	db := newTestDB(t)
	fed := NewLocalFederator(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	window, step := time.Hour, 5*time.Minute

	var samples []Sample
	for ai, app := range []string{"web", "api", "idle"} {
		if app == "idle" {
			continue
		}
		for i := 0; i < 40; i++ {
			ts := now.Add(-time.Duration(i) * 90 * time.Second)
			res := "service:" + app
			samples = append(samples,
				Sample{ResourceID: res, Metric: MetricHTTPRequests, Timestamp: ts, Value: float64(10 + ai + i%7)},
				Sample{ResourceID: res, Metric: MetricHTTPResponses5xx, Timestamp: ts, Value: float64(i % 3)},
				Sample{ResourceID: res, Metric: MetricHTTPResponses4xx, Timestamp: ts, Value: 1},
				Sample{ResourceID: res, Metric: LatencyBucketMetric(i % LatencyBucketCount), Timestamp: ts, Value: float64(5 + ai)},
			)
		}
	}
	if err := db.WriteSamples(ctx, samples); err != nil {
		t.Fatalf("seed: %v", err)
	}

	apps := []string{"web", "api", "idle"}
	got, err := RequestOverviews(ctx, fed, apps, window, step, now)
	if err != nil {
		t.Fatalf("RequestOverviews: %v", err)
	}
	for _, app := range apps {
		wantSum, err := SummarizeRequests(ctx, fed, app, window, now)
		if err != nil {
			t.Fatalf("SummarizeRequests(%s): %v", app, err)
		}
		wantPts, err := QueryRequests(ctx, fed, app, now.Add(-window), now, step)
		if err != nil {
			t.Fatalf("QueryRequests(%s): %v", app, err)
		}
		if !reflect.DeepEqual(got[app].Summary, wantSum) {
			t.Errorf("%s summary = %+v, want %+v", app, got[app].Summary, wantSum)
		}
		if len(got[app].Points) != len(wantPts) {
			t.Fatalf("%s points = %d, want %d", app, len(got[app].Points), len(wantPts))
		}
		for i := range wantPts {
			if !reflect.DeepEqual(got[app].Points[i], wantPts[i]) {
				t.Errorf("%s point %d = %+v, want %+v", app, i, got[app].Points[i], wantPts[i])
			}
		}
	}
	if got["idle"].Summary.HasTraffic {
		t.Error("idle app reported traffic")
	}
}

type plainMetricsQuerier struct{ f *Federator }

func (p plainMetricsQuerier) QueryMetrics(ctx context.Context, r, m string, from, to time.Time) ([]Sample, error) {
	return p.f.QueryMetrics(ctx, r, m, from, to)
}

func TestRequestOverviews_FallbackWithoutBatchSource(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := db.WriteSamples(ctx, []Sample{
		{ResourceID: "service:web", Metric: MetricHTTPRequests, Timestamp: now.Add(-time.Minute), Value: 30},
	}); err != nil {
		t.Fatal(err)
	}
	fed := NewLocalFederator(db)
	batched, _ := RequestOverviews(ctx, fed, []string{"web"}, time.Hour, 5*time.Minute, now)
	plain, _ := RequestOverviews(ctx, plainMetricsQuerier{fed}, []string{"web"}, time.Hour, 5*time.Minute, now)
	if !reflect.DeepEqual(batched, plain) {
		t.Errorf("batched %+v != fallback %+v", batched, plain)
	}
	if !plain["web"].Summary.HasTraffic {
		t.Error("fallback lost traffic")
	}
}

func TestQueryMany_ManySeriesOneCall(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	var seed []Sample
	for r := 0; r < 3; r++ {
		for m := 0; m < 2; m++ {
			seed = append(seed, Sample{ResourceID: fmt.Sprintf("service:r%d", r), Metric: fmt.Sprintf("m%d", m), Timestamp: now.Add(-time.Minute), Value: float64(r*10 + m)})
		}
	}
	if err := db.WriteSamples(ctx, seed); err != nil {
		t.Fatal(err)
	}
	got, err := db.QueryMany(ctx, []string{"service:r0", "service:r2"}, []string{"m1"}, now.Add(-time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Value != 1 || got[1].Value != 21 {
		t.Errorf("got %+v", got)
	}
}
