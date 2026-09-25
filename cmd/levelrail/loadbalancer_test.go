package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/loadbalancer"
)

type stubLBStats struct {
	stats map[string]loadbalancer.Stats
	err   error
}

func (s stubLBStats) UpstreamStats(context.Context) (map[string]loadbalancer.Stats, error) {
	return s.stats, s.err
}

func TestLBTelemetrySamples(t *testing.T) {
	reg := loadbalancer.NewRegistry()
	if got := lbTelemetrySamples(context.Background(), reg, stubLBStats{}); got != nil {
		t.Fatalf("empty registry samples = %v, want nil", got)
	}
	reg.Record(loadbalancer.Observation{
		Service: "web", ObservedAt: time.Now(),
		Upstreams: []loadbalancer.UpstreamObservation{
			{Upstream: loadbalancer.Upstream{Dial: "a:1"}, Running: true},
			{Upstream: loadbalancer.Upstream{Dial: "b:1"}, Running: true},
			{Upstream: loadbalancer.Upstream{Dial: "c:1"}, Running: false},
		},
	})

	tests := []struct {
		name        string
		stats       stubLBStats
		wantActive  int
		wantHealthy int
	}{
		{"with counters", stubLBStats{stats: map[string]loadbalancer.Stats{"a:1": {NumRequests: 2}, "b:1": {NumRequests: 3}}}, 5, 2},
		{"stats unavailable still reports health", stubLBStats{err: errors.New("admin down")}, 0, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lbTelemetrySamples(context.Background(), reg, tt.stats)
			if len(got) != 1 || got[0].Total != 3 || got[0].Healthy != tt.wantHealthy || got[0].ActiveRequests != tt.wantActive {
				t.Fatalf("samples = %+v", got)
			}
		})
	}
}
