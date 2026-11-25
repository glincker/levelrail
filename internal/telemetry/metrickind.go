package telemetry

import "strings"

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

// KindOf returns the kind for a metric name, with gauge as the fallback.
func KindOf(metric string) MetricKind {
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
