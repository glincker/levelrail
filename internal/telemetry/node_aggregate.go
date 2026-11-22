package telemetry

import (
	"sort"
	"time"
)

// SumAcrossResources collapses samples from multiple resources (e.g.
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
