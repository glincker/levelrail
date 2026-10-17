package telemetry

import (
	"context"
	"time"
)

// Autoscaler gauge names, one sample per autoscaled app per controller tick.
const (
	MetricAutoscaleCurrent    = "autoscale_replicas_current"
	MetricAutoscaleDesired    = "autoscale_replicas_desired"
	MetricAutoscaleNoCapacity = "autoscale_no_capacity"
	MetricAutoscaleFlaps1h    = "autoscale_flaps_1h"
)

// AutoscaleSample is one app's autoscaler snapshot.
type AutoscaleSample struct {
	App        string
	Current    int
	Desired    int
	NoCapacity bool
	Flaps      int
}

// RecordAutoscale writes the autoscaler gauges for each sample, so a plain
// threshold alert rule can watch NoCapacity and flapping.
func (db *DB) RecordAutoscale(ctx context.Context, samples []AutoscaleSample, at time.Time) error {
	rows := make([]Sample, 0, len(samples)*4)
	for _, s := range samples {
		id := "service:" + s.App
		noCap := 0.0
		if s.NoCapacity {
			noCap = 1
		}
		rows = append(rows,
			Sample{ResourceID: id, Metric: MetricAutoscaleCurrent, Timestamp: at, Value: float64(s.Current)},
			Sample{ResourceID: id, Metric: MetricAutoscaleDesired, Timestamp: at, Value: float64(s.Desired)},
			Sample{ResourceID: id, Metric: MetricAutoscaleNoCapacity, Timestamp: at, Value: noCap},
			Sample{ResourceID: id, Metric: MetricAutoscaleFlaps1h, Timestamp: at, Value: float64(s.Flaps)},
		)
	}
	if len(rows) == 0 {
		return nil
	}
	return db.WriteSamples(ctx, rows)
}
