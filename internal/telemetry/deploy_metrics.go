package telemetry

import (
	"context"
	"time"
)

// Metric names for TASKS.md 2.1's remaining gap: deploy frequency and
// build duration. Unlike every other metric in this store, neither comes
// from a Docker stats poll (collector.go's sampleValues): a deploy or a
// build is a discrete event, not a continuously observable resource, so
// each is recorded once, at the moment it happens, by its own caller
// (internal/reconcile/application for a deploy cutover,
// internal/deploy for a completed build) rather than by Collector.
const (
	// MetricDeployCount is one sample per completed deploy cutover
	// (RecordDeploy), always Value 1. A raw sample count therefore *is*
	// deploy frequency for a range: Aggregate's own Count field (how
	// many raw samples fell in a bucket) already answers "how many
	// deploys in this window" with no separate counter or rate
	// computation needed.
	MetricDeployCount = "deploy_count"
	// MetricBuildDuration is one sample per completed build
	// (RecordBuildDuration), Value the build's wall-clock duration in
	// seconds, taken directly from build.Result.Duration rather than
	// re-measured here.
	MetricBuildDuration = "build_duration_seconds"
)

// RecordDeploy writes one MetricDeployCount sample for serviceName at at.
// Callers must call this only when a deploy actually happened (a new
// container was created/started and passed its readiness probe), not on
// every reconcile tick that finds nothing to do: internal/reconcile/
// application.Controller's own "Deployed" vs "AlreadyRunning" distinction
// is exactly that signal.
func (db *DB) RecordDeploy(ctx context.Context, serviceName string, at time.Time) error {
	return db.WriteSamples(ctx, []Sample{{
		ResourceID: "service:" + serviceName,
		Metric:     MetricDeployCount,
		Timestamp:  at,
		Value:      1,
	}})
}

// RecordBuildDuration writes one MetricBuildDuration sample for
// serviceName at at.
func (db *DB) RecordBuildDuration(ctx context.Context, serviceName string, d time.Duration, at time.Time) error {
	return db.WriteSamples(ctx, []Sample{{
		ResourceID: "service:" + serviceName,
		Metric:     MetricBuildDuration,
		Timestamp:  at,
		Value:      d.Seconds(),
	}})
}
