package telemetry

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeMetricsSource struct {
	samples []Sample
	err     error
}

func (f *fakeMetricsSource) Query(_ context.Context, _, _ string, _, _ time.Time) ([]Sample, error) {
	return f.samples, f.err
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
