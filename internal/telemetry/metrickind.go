package telemetry

import (
	"strings"
	"testing"
)

// MetricKind says how a metric's raw values combine in a rollup bucket.
type MetricKind int

const (
	// KindGauge is a level at sample time: rollups keep sum and count so the mean is sum/count.
	KindGauge MetricKind = iota
	// KindDelta is an amount since the last tick: rollups add the values.
	KindDelta
	// KindCumulative is a running total: rollups store the reset aware increase.
	KindCumulative
)

// cumulativeMetrics are the running totals; everything else is a gauge unless it is a delta.
var cumulativeMetrics = []string{"network_rx_bytes", "network_tx_bytes", "disk_read_bytes", "disk_write_bytes"}

// Database metric names written by internal/dbstats. Every db_* name must be listed in dbMetricKinds.
const (
	MetricDBConnActive   = "db_connections_active"
	MetricDBConnIdle     = "db_connections_idle"
	MetricDBConnTotal    = "db_connections_total"
	MetricDBConnMax      = "db_connections_max"
	MetricDBBlksHit      = "db_blks_hit"
	MetricDBBlksRead     = "db_blks_read"
	MetricDBTupInserted  = "db_tup_inserted"
	MetricDBTupUpdated   = "db_tup_updated"
	MetricDBTupDeleted   = "db_tup_deleted"
	MetricDBDeadlocks    = "db_deadlocks"
	MetricDBXactCommit   = "db_xact_commit"
	MetricDBXactRollback = "db_xact_rollback"
	MetricDBLocksWaiting = "db_locks_waiting"
	// MetricDBSizePrefix is followed by the database name: one gauge series per database.
	MetricDBSizePrefix = "db_size_bytes."
)

// dbMetricKinds registers every fixed db_* metric; counters are written as per tick deltas, the rest are gauges.
var dbMetricKinds = map[string]MetricKind{
	MetricDBConnActive:   KindGauge,
	MetricDBConnIdle:     KindGauge,
	MetricDBConnTotal:    KindGauge,
	MetricDBConnMax:      KindGauge,
	MetricDBBlksHit:      KindDelta,
	MetricDBBlksRead:     KindDelta,
	MetricDBTupInserted:  KindDelta,
	MetricDBTupUpdated:   KindDelta,
	MetricDBTupDeleted:   KindDelta,
	MetricDBDeadlocks:    KindDelta,
	MetricDBXactCommit:   KindDelta,
	MetricDBXactRollback: KindDelta,
	MetricDBLocksWaiting: KindGauge,
}

// IsDBMetric reports whether the name is registered, including per database size series.
func IsDBMetric(metric string) bool {
	if _, ok := dbMetricKinds[metric]; ok {
		return true
	}
	return strings.HasPrefix(metric, MetricDBSizePrefix) && len(metric) > len(MetricDBSizePrefix)
}

// KindOf returns the kind for a metric name, with gauge as the fallback.
func KindOf(metric string) MetricKind {
	if k, ok := dbMetricKinds[metric]; ok {
		return k
	}
	if strings.HasPrefix(metric, MetricDBSizePrefix) {
		return KindGauge
	}
	// A new db_ series must be registered; tests fail loudly instead of rolling it up as a gauge.
	if strings.HasPrefix(metric, "db_") && !IsDBMetric(metric) && testing.Testing() {
		panic("telemetry: unregistered db metric " + metric)
	}
	for _, name := range cumulativeMetrics {
		if metric == name {
			return KindCumulative
		}
	}
	if strings.HasPrefix(metric, metricHTTPPrefix) {
		return KindDelta
	}
	return KindGauge
}

// IsSumMetric reports whether rollups must add the metric's values.
func IsSumMetric(metric string) bool { return KindOf(metric) == KindDelta }

// cumulativeIn returns an SQL placeholder list and args for the cumulative metric names.
func cumulativeIn() (string, []any) {
	args := make([]any, len(cumulativeMetrics))
	for i, name := range cumulativeMetrics {
		args[i] = name
	}
	return "(?" + strings.Repeat(", ?", len(args)-1) + ")", args
}
