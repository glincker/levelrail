package telemetry

import (
	"context"
	"errors"
	"sort"
	"time"
)

// MetricsSource is what the query layer needs from one agent, local or
// remote, to answer a metrics query. *DB satisfies this today (the
// single-node, in-process "agent"); a Phase 3 gRPC client would satisfy
// it too without Federator's callers (internal/api) ever changing,
// exactly the precedent CLAUDE.md 4.3 already sets for the reconcile
// agent transport ("must also work in single-node mode... communicating
// over an in-memory transport that implements the same interface").
type MetricsSource interface {
	Query(ctx context.Context, resourceID, metric string, from, to time.Time) ([]Sample, error)
}

// LogsSource is the log-query equivalent of MetricsSource.
type LogsSource interface {
	QueryLogs(ctx context.Context, resourceID string, from, to time.Time, query string) ([]LogEntry, error)
}

// Federator fans a query out to every configured source and merges
// results, per ADR 008: "the control plane fans queries out to agents
// and merges results at query time; query is pull, not push." Today
// there is exactly one source (this node's own local DB), so federation
// is a merge-of-one; the shape is what Phase 3 fills in with real
// per-node sources.
type Federator struct {
	metrics []MetricsSource
	logs    []LogsSource
}

// NewFederator builds a Federator over an explicit source list, for
// Phase 3 once real per-node sources exist alongside (or instead of) the
// local one.
func NewFederator(metrics []MetricsSource, logs []LogsSource) *Federator {
	return &Federator{metrics: metrics, logs: logs}
}

// NewLocalFederator builds a Federator with exactly one source: this
// process's own local DB. The single-node shape every control plane
// runs today, until Phase 3 adds real remote agents.
func NewLocalFederator(db *DB) *Federator {
	return NewFederator([]MetricsSource{db}, []LogsSource{db})
}

// QueryMetrics fans out to every metrics source and merges results,
// sorted by timestamp ascending. A source erroring doesn't fail the
// whole query: ADR 008's Consequences require the query layer to
// "handle partial results gracefully... deciding what the dashboard
// shows when [an agent is unreachable]," so this returns whatever the
// healthy sources answered, plus the errors as a joined error the
// caller can log or surface as a partial-result warning, not use to
// discard everything.
func (f *Federator) QueryMetrics(ctx context.Context, resourceID, metric string, from, to time.Time) ([]Sample, error) {
	var all []Sample
	var errs []error
	for _, src := range f.metrics {
		got, err := src.Query(ctx, resourceID, metric, from, to)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		all = append(all, got...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Timestamp.Before(all[j].Timestamp) })
	return all, errors.Join(errs...)
}

// QueryLogs is QueryMetrics' log-query equivalent, merged and sorted by
// timestamp ascending the same way.
func (f *Federator) QueryLogs(ctx context.Context, resourceID string, from, to time.Time, query string) ([]LogEntry, error) {
	var all []LogEntry
	var errs []error
	for _, src := range f.logs {
		got, err := src.QueryLogs(ctx, resourceID, from, to, query)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		all = append(all, got...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Timestamp.Before(all[j].Timestamp) })
	return all, errors.Join(errs...)
}

// AggregatedPoint is one bucketed value from Aggregate.
type AggregatedPoint struct {
	// Timestamp is the bucket's start, not a sample's own timestamp.
	Timestamp time.Time
	// Value is the average of every sample that fell in this bucket.
	Value float64
	// Count is how many raw samples contributed, so a caller (or a
	// future frontend) can tell a bucket built from one sample from one
	// built from sixty, without a separate query.
	Count int
}

// Aggregate buckets samples (assumed already timestamp-ascending, which
// Federator.QueryMetrics already guarantees) into fixed-width windows
// starting at from, each bucket's value the average of the samples
// inside it. step <= 0 means "no aggregation," one point per sample.
// An empty bucket is omitted, not returned as a zero: a sparse series
// (a container that stopped reporting for an hour) should read as a gap
// in the chart, not a real reading of zero.
func Aggregate(samples []Sample, from time.Time, step time.Duration) []AggregatedPoint {
	if step <= 0 {
		out := make([]AggregatedPoint, len(samples))
		for i, s := range samples {
			out[i] = AggregatedPoint{Timestamp: s.Timestamp, Value: s.Value, Count: 1}
		}
		return out
	}

	type bucket struct {
		sum   float64
		count int
	}
	buckets := make(map[int64]*bucket)
	var order []int64

	for _, s := range samples {
		idx := int64(s.Timestamp.Sub(from) / step)
		b, ok := buckets[idx]
		if !ok {
			b = &bucket{}
			buckets[idx] = b
			order = append(order, idx)
		}
		b.sum += s.Value
		b.count++
	}

	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })

	out := make([]AggregatedPoint, len(order))
	for i, idx := range order {
		b := buckets[idx]
		out[i] = AggregatedPoint{
			Timestamp: from.Add(time.Duration(idx) * step),
			Value:     b.sum / float64(b.count),
			Count:     b.count,
		}
	}
	return out
}
