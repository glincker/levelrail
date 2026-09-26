package main

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type fakeLatest map[string][]telemetry.Sample

func (f fakeLatest) LatestByMetric(_ context.Context, metric string) ([]telemetry.Sample, error) {
	return f[metric], nil
}

func TestNodeDiskFacts(t *testing.T) {
	now := time.Now()
	fresh := now.Add(-time.Minute)
	stale := now.Add(-time.Hour)
	mk := func(ts time.Time, used, total float64) fakeLatest {
		return fakeLatest{
			telemetry.MetricDiskUsedBytes:  {{ResourceID: "node:n1", Timestamp: ts, Value: used}},
			telemetry.MetricDiskTotalBytes: {{ResourceID: "node:n1", Timestamp: ts, Value: total}},
		}
	}
	tests := []struct {
		name     string
		src      fakeLatest
		node     string
		wantFree int64
		wantOK   bool
	}{
		{"fresh", mk(fresh, 30, 100), "n1", 70, true},
		{"stale is unknown", mk(stale, 30, 100), "n1", 0, false},
		{"other node is unknown", mk(fresh, 30, 100), "n2", 0, false},
		{"no samples", fakeLatest{}, "n1", 0, false},
		{"used above total clamps to zero", mk(fresh, 120, 100), "n1", 0, true},
		{"zero total is unknown", mk(fresh, 0, 0), "n1", 0, false},
	}
	for _, tt := range tests {
		free, _, ok := nodeDiskFacts(context.Background(), tt.src, tt.node, 10*time.Minute, now)
		if free != tt.wantFree || ok != tt.wantOK {
			t.Errorf("%s: free=%d ok=%v, want %d %v", tt.name, free, ok, tt.wantFree, tt.wantOK)
		}
	}
}
