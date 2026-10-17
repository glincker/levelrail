package alerting

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Derived request metrics a KindThreshold rule can watch. They are computed
// from the ingress request counters over a trailing window, not stored.
const (
	MetricRequestErrorRate5xxPct = "request_error_rate_5xx_pct"
	MetricRequestP95Ms           = "request_p95_ms"
	MetricRequestP99Ms           = "request_p99_ms"

	// EnvRequestSignalWindow overrides the trailing window of derived request metrics.
	EnvRequestSignalWindow = "APP_ALERT_REQUEST_WINDOW"

	defaultRequestSignalWindow = 5 * time.Minute
)

// IsRequestSignal reports whether metric is a derived request metric.
func IsRequestSignal(metric string) bool {
	switch metric {
	case MetricRequestErrorRate5xxPct, MetricRequestP95Ms, MetricRequestP99Ms:
		return true
	}
	return false
}

func requestSignalWindow() time.Duration {
	if d, err := time.ParseDuration(os.Getenv(EnvRequestSignalWindow)); err == nil && d > 0 {
		return d
	}
	return defaultRequestSignalWindow
}

// requestSignalValue computes a derived metric for the rule's app. ok is
// false when the window had no traffic, which neither confirms nor denies
// the condition.
func requestSignalValue(ctx context.Context, source MetricsSource, r Rule, now time.Time) (value float64, ok bool, err error) {
	app := strings.TrimPrefix(r.ResourceID, "service:")
	window := requestSignalWindow()
	points, err := telemetry.QueryRequests(ctx, source, app, now.Add(-window), now, window+time.Second)
	if err != nil && len(points) == 0 {
		return 0, false, err
	}
	if len(points) == 0 {
		return 0, false, nil
	}
	p := points[0]
	switch r.Metric {
	case MetricRequestErrorRate5xxPct:
		return p.ErrorRate5xx * 100, true, nil
	case MetricRequestP95Ms:
		return p.P95Ms, true, nil
	default:
		return p.P99Ms, true, nil
	}
}
