package telemetry

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeMetricsSource struct {
	samples []Sample
	err     error
	latest  []Sample
	// latestErr is separate from err: a test exercising QueryMetrics's
	// error path should not accidentally also fail LatestByMetric calls
	// it never intended to make, and vice versa.
	latestErr error
}

func (f *fakeMetricsSource) Query(_ context.Context, _, _ string, _, _ time.Time) ([]Sample, error) {
	return f.samples, f.err
}

func (f *fakeMetricsSource) LatestByMetric(_ context.Context, _ string) ([]Sample, error) {
	return f.latest, f.latestErr
}

type fakeLogsSource struct {
	entries []LogEntry
	err     error
}

func (f *fakeLogsSource) QueryLogs(_ context.Context, _ string, _, _ time.Time, _ string) ([]LogEntry, error) {
	return f.entries, f.err
}

func TestFederator_QueryMetrics_MergesAndSorts(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	src1 := &fakeMetricsSource{samples: []Sample{{Timestamp: base.Add(2 * time.Second), Value: 2}}}
	src2 := &fakeMetricsSource{samples: []Sample{{Timestamp: base, Value: 1}, {Timestamp: base.Add(4 * time.Second), Value: 4}}}

	f := NewFederator([]MetricsSource{src1, src2}, nil)
	got, err := f.QueryMetrics(context.Background(), "service:web", "cpu_percent", base, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("QueryMetrics() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("QueryMetrics() = %d samples, want 3 (merged across both sources)", len(got))
	}
	for i := 0; i < len(got)-1; i++ {
		if got[i].Timestamp.After(got[i+1].Timestamp) {
			t.Errorf("results not sorted ascending: %v after %v", got[i].Timestamp, got[i+1].Timestamp)
		}
	}
}

func TestFederator_QueryMetrics_OneSourceErrors_OthersStillReturned(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	healthy := &fakeMetricsSource{samples: []Sample{{Timestamp: base, Value: 1}}}
	broken := &fakeMetricsSource{err: errors.New("agent unreachable")}

	f := NewFederator([]MetricsSource{healthy, broken}, nil)
	got, err := f.QueryMetrics(context.Background(), "service:web", "cpu_percent", base, base.Add(time.Hour))
	if err == nil {
		t.Fatal("QueryMetrics() error = nil, want the broken source's error surfaced")
	}
	if len(got) != 1 {
		t.Errorf("QueryMetrics() = %d samples, want 1 (the healthy source's result), per ADR 008's partial-results requirement", len(got))
	}
}

func TestFederator_QueryLogs_MergesAndSorts(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	src1 := &fakeLogsSource{entries: []LogEntry{{Timestamp: base.Add(time.Second), Message: "b"}}}
	src2 := &fakeLogsSource{entries: []LogEntry{{Timestamp: base, Message: "a"}}}

	f := NewFederator(nil, []LogsSource{src1, src2})
	got, err := f.QueryLogs(context.Background(), "service:web", base, base.Add(time.Hour), "")
	if err != nil {
		t.Fatalf("QueryLogs() error = %v", err)
	}
	if len(got) != 2 || got[0].Message != "a" || got[1].Message != "b" {
		t.Errorf("QueryLogs() = %+v, want [a, b] in timestamp order", got)
	}
}

func TestFederator_LatestByMetric_MergesByResourceKeepingNewest(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	// service:web reported by both sources at different times: the
	// newer one (src2's) must win, not whichever source happened first.
	src1 := &fakeMetricsSource{latest: []Sample{
		{ResourceID: "service:web", Timestamp: base, Value: 10},
		{ResourceID: "service:worker", Timestamp: base, Value: 5},
	}}
	src2 := &fakeMetricsSource{latest: []Sample{
		{ResourceID: "service:web", Timestamp: base.Add(time.Minute), Value: 40},
	}}

	f := NewFederator([]MetricsSource{src1, src2}, nil)
	got, err := f.LatestByMetric(context.Background(), "cpu_percent")
	if err != nil {
		t.Fatalf("LatestByMetric() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("LatestByMetric() = %d resources, want 2", len(got))
	}
	byResource := make(map[string]Sample, len(got))
	for _, s := range got {
		byResource[s.ResourceID] = s
	}
	if s := byResource["service:web"]; s.Value != 40 {
		t.Errorf("service:web = %+v, want the newer sample (40)", s)
	}
	if s := byResource["service:worker"]; s.Value != 5 {
		t.Errorf("service:worker = %+v, want 5", s)
	}
}

func TestFederator_LatestByMetric_OneSourceErrors_OthersStillReturned(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	healthy := &fakeMetricsSource{latest: []Sample{{ResourceID: "service:web", Timestamp: base, Value: 1}}}
	broken := &fakeMetricsSource{latestErr: errors.New("agent unreachable")}

	f := NewFederator([]MetricsSource{healthy, broken}, nil)
	got, err := f.LatestByMetric(context.Background(), "cpu_percent")
	if err == nil {
		t.Fatal("LatestByMetric() error = nil, want the broken source's error surfaced")
	}
	if len(got) != 1 {
		t.Errorf("LatestByMetric() = %d resources, want 1 (the healthy source's result)", len(got))
	}
}

func TestNewLocalFederator_UsesTheSameDBForBoth(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	if err := db.WriteSamples(ctx, []Sample{{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: base, Value: 5}}); err != nil {
		t.Fatalf("WriteSamples() error = %v", err)
	}

	f := NewLocalFederator(db)
	got, err := f.QueryMetrics(ctx, "service:web", "cpu_percent", base.Add(-time.Minute), base.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryMetrics() error = %v", err)
	}
	if len(got) != 1 || got[0].Value != 5 {
		t.Errorf("QueryMetrics() via NewLocalFederator = %+v, want the sample written directly to db", got)
	}
}

func TestAggregate_NoStep_OnePointPerSample(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	samples := []Sample{{Timestamp: base, Value: 1}, {Timestamp: base.Add(time.Second), Value: 2}}

	got := Aggregate(samples, base, 0)
	if len(got) != 2 {
		t.Fatalf("Aggregate(step=0) = %d points, want 2 (no bucketing)", len(got))
	}
	if got[0].Count != 1 || got[1].Count != 1 {
		t.Errorf("Aggregate(step=0) counts = [%d, %d], want [1, 1]", got[0].Count, got[1].Count)
	}
}

func TestAggregate_BucketsAndAverages(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	samples := []Sample{
		{Timestamp: base, Value: 10},                        // bucket 0
		{Timestamp: base.Add(10 * time.Second), Value: 20},  // bucket 0 (step=60s)
		{Timestamp: base.Add(70 * time.Second), Value: 100}, // bucket 1
	}

	got := Aggregate(samples, base, 60*time.Second)
	if len(got) != 2 {
		t.Fatalf("Aggregate() = %d buckets, want 2", len(got))
	}
	if got[0].Value != 15 || got[0].Count != 2 {
		t.Errorf("bucket 0 = {Value: %v, Count: %d}, want {Value: 15, Count: 2}", got[0].Value, got[0].Count)
	}
	if got[1].Value != 100 || got[1].Count != 1 {
		t.Errorf("bucket 1 = {Value: %v, Count: %d}, want {Value: 100, Count: 1}", got[1].Value, got[1].Count)
	}
	if !got[1].Timestamp.Equal(base.Add(60 * time.Second)) {
		t.Errorf("bucket 1 timestamp = %v, want %v (bucket start, not sample time)", got[1].Timestamp, base.Add(60*time.Second))
	}
}

func TestAggregate_EmptyBucketsOmitted(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	// A gap: nothing in bucket 1 (60s-120s), a real sample in bucket 2.
	samples := []Sample{
		{Timestamp: base, Value: 1},
		{Timestamp: base.Add(130 * time.Second), Value: 2},
	}

	got := Aggregate(samples, base, 60*time.Second)
	if len(got) != 2 {
		t.Fatalf("Aggregate() = %d buckets, want 2 (the empty middle bucket must be omitted, not returned as zero)", len(got))
	}
}

type barrierMetricsSource struct {
	started *sync.WaitGroup
	release chan struct{}
	samples []Sample
	err     error
}

func (b *barrierMetricsSource) Query(ctx context.Context, _, _ string, _, _ time.Time) ([]Sample, error) {
	b.started.Done()
	select {
	case <-b.release:
		return b.samples, b.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (b *barrierMetricsSource) LatestByMetric(_ context.Context, _ string) ([]Sample, error) {
	return nil, nil
}

func TestFederator_QueriesSourcesConcurrently(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	started := &sync.WaitGroup{}
	started.Add(3)
	release := make(chan struct{})
	srcs := []MetricsSource{
		&barrierMetricsSource{started: started, release: release, samples: []Sample{{ResourceID: "a", Timestamp: base.Add(2 * time.Second)}}},
		&barrierMetricsSource{started: started, release: release, err: errors.New("node-2 down")},
		&barrierMetricsSource{started: started, release: release, samples: []Sample{{ResourceID: "c", Timestamp: base}}},
	}
	f := NewFederator(srcs, nil)

	// Every source blocks until all three have started, so a sequential
	// fan-out would never reach the release and the query would time out.
	go func() {
		started.Wait()
		close(release)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := f.QueryMetrics(ctx, "r", "cpu_percent", base, base.Add(time.Hour))
	if err == nil || !strings.Contains(err.Error(), "node-2 down") {
		t.Fatalf("QueryMetrics() error = %v, want the failing source's error surfaced", err)
	}
	if len(got) != 2 || got[0].ResourceID != "c" || got[1].ResourceID != "a" {
		t.Errorf("QueryMetrics() = %+v, want the two healthy results merged in timestamp order", got)
	}
}

func TestFederator_ErrorOrderFollowsSourceOrder(t *testing.T) {
	srcs := []MetricsSource{
		&fakeMetricsSource{latestErr: errors.New("first")},
		&fakeMetricsSource{latestErr: errors.New("second")},
		&fakeMetricsSource{latestErr: errors.New("third")},
	}
	f := NewFederator(srcs, nil)
	for i := 0; i < 20; i++ {
		_, err := f.LatestByMetric(context.Background(), "cpu_percent")
		if err == nil || err.Error() != "first\nsecond\nthird" {
			t.Fatalf("LatestByMetric() error = %v, want errors joined in source order", err)
		}
	}
}
