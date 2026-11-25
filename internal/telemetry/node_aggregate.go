package telemetry

import (
	"sort"
	"time"
)

// SumAcrossResources collapses samples from multiple resources (e.g.
func SumAcrossResources(nodeResourceID, metric string, samples []Sample) []Sample {
	type bucket struct {
		sum    float64
		lo, hi float64
		count  int64
		rollup bool
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
		lo, hi := s.Value, s.Value
		if s.Count > 0 {
			lo, hi = s.Min, s.Max
			b.rollup = true
			// Resources are summed, so the weight is the widest input, not the total.
			b.count = max(b.count, s.Count)
		}
		b.sum += s.Value
		b.lo += lo
		b.hi += hi
	}

	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })

	out := make([]Sample, len(order))
	for i, key := range order {
		b := buckets[key]
		out[i] = Sample{
			ResourceID: nodeResourceID,
			Metric:     metric,
			Timestamp:  time.Unix(key, 0).UTC(),
			Value:      b.sum,
		}
		if b.rollup {
			out[i].Count, out[i].Sum = b.count, b.sum*float64(b.count)
			out[i].Min, out[i].Max = b.lo, b.hi
		}
	}
	return out
}
