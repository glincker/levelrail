package telemetry

import (
	"sort"
	"time"
)

// SumAcrossResources collapses samples from multiple resources (e.g.
// every container currently placed on one node) for a single metric
// into one per-timestamp total: "how much of this metric was in use,
// summed across everything running here, at each collection tick."
//
// Samples are grouped by their exact Timestamp, not a time bucket:
// Collector.CollectOnce (collector.go) captures one `now` per tick and
// stamps every target polled in that tick with it, and
// WriteSamples/Query round-trip that value at one-second precision
// (store.go), so two containers collected in the same tick share the
// exact same Timestamp here, not merely an approximately-close one. A
// caller that wants coarser buckets afterward (a chart's `step` query
// param) runs this function's output through Aggregate, the same as a
// single resource's own series already does
// (internal/api/metrics.go's handleQueryMetrics).
//
// Mixing metrics in one call would produce a meaningless sum (adding a
// percent to a byte count), so this assumes every sample passed in
// already shares one metric; callers query one metric per resource and
// concatenate before calling this, matching Federator.QueryMetrics' own
// "one metric per call" contract.
//
// The returned samples' ResourceID and Metric are the caller-supplied
// nodeResourceID/metric, not copied from any individual input sample:
// the result represents a virtual, computed-on-the-fly aggregate
// resource, not any single container, and is never itself written back
// to the store.
//
// Two known, deliberately accepted limitations, not bugs:
//
//   - The exact-Timestamp grouping this function relies on holds only
//     because everything collected today runs through one process's own
//     Collector.CollectOnce, sharing one clock. TASKS.md's Phase 3 plan
//     (per-node agents, each with an independent local collector, CLAUDE.md
//     4.8) breaks this assumption: two agents' clocks are never perfectly
//     synchronized, so their samples for "the same tick" won't share an
//     identical Unix second. Revisit this grouping (a tolerance window,
//     not exact equality) before this function is ever fed cross-agent
//     samples, not after.
//   - A bucket is never flagged as partially-covered: if one resource has
//     a sample at a given timestamp and another (also placed on this
//     node for that whole range) doesn't, the sum silently reflects only
//     the resource that reported, indistinguishable here from "every
//     resource genuinely totaled this." Neither zero-filled nor crashed,
//     both worse options, but a caller cannot currently tell "real total"
//     from "partial total" from this function's output alone.
func SumAcrossResources(nodeResourceID, metric string, samples []Sample) []Sample {
	type bucket struct {
		sum   float64
		count int
	}
	buckets := make(map[int64]*bucket)
	var order []int64

	for _, s := range samples {
		key := s.Timestamp.Unix()
		b, ok := buckets[key]
		if !ok {
			b = &bucket{}
			buckets[key] = b
			order = append(order, key)
		}
		b.sum += s.Value
		b.count++
	}

	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })

	out := make([]Sample, len(order))
	for i, key := range order {
		out[i] = Sample{
			ResourceID: nodeResourceID,
			Metric:     metric,
			Timestamp:  time.Unix(key, 0).UTC(),
			Value:      buckets[key].sum,
		}
	}
	return out
}
