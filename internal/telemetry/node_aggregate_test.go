package telemetry

import (
	"testing"
	"time"
)

func TestSumAcrossResources(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()

	tests := []struct {
		name    string
		samples []Sample
		want    []Sample
	}{
		{
			name:    "empty input yields empty output",
			samples: nil,
			want:    []Sample{},
		},
		{
			name: "single resource passes through unchanged in value",
			samples: []Sample{
				{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: base, Value: 12.5},
			},
			want: []Sample{
				{ResourceID: "node:n1", Metric: "cpu_percent", Timestamp: base, Value: 12.5},
			},
		},
		{
			name: "two resources at the same tick are summed",
			samples: []Sample{
				{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: base, Value: 10},
				{ResourceID: "service:worker", Metric: "cpu_percent", Timestamp: base, Value: 25},
			},
			want: []Sample{
				{ResourceID: "node:n1", Metric: "cpu_percent", Timestamp: base, Value: 35},
			},
		},
		{
			name: "distinct ticks stay distinct points, sorted ascending",
			samples: []Sample{
				{ResourceID: "service:web", Metric: "memory_usage_bytes", Timestamp: base.Add(2 * time.Second), Value: 200},
				{ResourceID: "service:worker", Metric: "memory_usage_bytes", Timestamp: base, Value: 100},
				{ResourceID: "service:web", Metric: "memory_usage_bytes", Timestamp: base, Value: 50},
			},
			want: []Sample{
				{ResourceID: "node:n1", Metric: "memory_usage_bytes", Timestamp: base, Value: 150},
				{ResourceID: "node:n1", Metric: "memory_usage_bytes", Timestamp: base.Add(2 * time.Second), Value: 200},
			},
		},
		{
			name: "a resource that only reported on some ticks doesn't zero-fill the others",
			samples: []Sample{
				{ResourceID: "service:web", Metric: "network_rx_bytes", Timestamp: base, Value: 10},
				{ResourceID: "service:web", Metric: "network_rx_bytes", Timestamp: base.Add(time.Second), Value: 20},
				{ResourceID: "service:worker", Metric: "network_rx_bytes", Timestamp: base, Value: 5},
			},
			want: []Sample{
				{ResourceID: "node:n1", Metric: "network_rx_bytes", Timestamp: base, Value: 15},
				{ResourceID: "node:n1", Metric: "network_rx_bytes", Timestamp: base.Add(time.Second), Value: 20},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric := "cpu_percent"
			if len(tt.want) > 0 {
				metric = tt.want[0].Metric
			}
			got := SumAcrossResources("node:n1", metric, tt.samples)
			if len(got) != len(tt.want) {
				t.Fatalf("SumAcrossResources() = %d points, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range got {
				if !got[i].Timestamp.Equal(tt.want[i].Timestamp) {
					t.Errorf("point %d timestamp = %v, want %v", i, got[i].Timestamp, tt.want[i].Timestamp)
				}
				if got[i].Value != tt.want[i].Value {
					t.Errorf("point %d value = %v, want %v", i, got[i].Value, tt.want[i].Value)
				}
				if got[i].ResourceID != tt.want[i].ResourceID {
					t.Errorf("point %d resourceID = %q, want %q", i, got[i].ResourceID, tt.want[i].ResourceID)
				}
				if got[i].Metric != tt.want[i].Metric {
					t.Errorf("point %d metric = %q, want %q", i, got[i].Metric, tt.want[i].Metric)
				}
			}
		})
	}
}

// TestSumAcrossResources_ChainsIntoAggregate is the real end-to-end
// shape handleQueryNodeMetrics uses: sum containers per tick, then bucket
// the summed series the same way a single resource's own series already
// does. Verifies the two functions compose correctly rather than testing
// either in isolation only.
func TestSumAcrossResources_ChainsIntoAggregate(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	samples := []Sample{
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: base, Value: 10},
		{ResourceID: "service:worker", Metric: "cpu_percent", Timestamp: base, Value: 20},
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: base.Add(70 * time.Second), Value: 30},
		{ResourceID: "service:worker", Metric: "cpu_percent", Timestamp: base.Add(70 * time.Second), Value: 40},
	}

	summed := SumAcrossResources("node:n1", "cpu_percent", samples)
	got := Aggregate(summed, base, 60*time.Second)

	if len(got) != 2 {
		t.Fatalf("Aggregate(summed) = %d buckets, want 2", len(got))
	}
	if got[0].Value != 30 {
		t.Errorf("bucket 0 = %v, want 30 (10+20)", got[0].Value)
	}
	if got[1].Value != 70 {
		t.Errorf("bucket 1 = %v, want 70 (30+40)", got[1].Value)
	}
}
