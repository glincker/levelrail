package telemetry

import (
	"context"
	"time"
)

// Load balancer metric names, one sample per service per sampling tick.
const (
	MetricLBUpstreamsTotal   = "lb_upstreams_total"
	MetricLBUpstreamsHealthy = "lb_upstreams_healthy"
	MetricLBActiveRequests   = "lb_active_requests"
)

// LoadBalancerSample is one service's balancer snapshot.
type LoadBalancerSample struct {
	Service        string
	Total          int
	Healthy        int
	ActiveRequests int
}

// RecordLoadBalancer writes the three load balancer gauges for each sample.
func (db *DB) RecordLoadBalancer(ctx context.Context, samples []LoadBalancerSample, at time.Time) error {
	rows := make([]Sample, 0, len(samples)*3)
	for _, s := range samples {
		id := "service:" + s.Service
		rows = append(rows,
			Sample{ResourceID: id, Metric: MetricLBUpstreamsTotal, Timestamp: at, Value: float64(s.Total)},
			Sample{ResourceID: id, Metric: MetricLBUpstreamsHealthy, Timestamp: at, Value: float64(s.Healthy)},
			Sample{ResourceID: id, Metric: MetricLBActiveRequests, Timestamp: at, Value: float64(s.ActiveRequests)},
		)
	}
	if len(rows) == 0 {
		return nil
	}
	return db.WriteSamples(ctx, rows)
}
