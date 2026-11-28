package api

import (
	"context"
	"fmt"

	"github.com/glincker/levelrail/internal/runtimesettings"
)

// KeyTrafficSlowUpstreamMs is the p95 latency above which the domain page flags the app's upstream as slow.
const KeyTrafficSlowUpstreamMs = "traffic.slow_upstream_ms"

const (
	defaultSlowUpstreamMs = 1500
	minSlowUpstreamMs     = 50
	maxSlowUpstreamMs     = 60000
	// slowUpstreamMinRequests keeps a handful of requests from raising the hint.
	slowUpstreamMinRequests = 20
	slowUpstreamRouteCount  = 3
)

func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyTrafficSlowUpstreamMs, Category: runtimesettings.CategoryObservability, Kind: runtimesettings.KindInt,
		Default: fmt.Sprint(defaultSlowUpstreamMs), Apply: runtimesettings.ApplyLive,
		CheckValue: func(canonical string) error {
			var n int
			if _, err := fmt.Sscanf(canonical, "%d", &n); err != nil || n < minSlowUpstreamMs || n > maxSlowUpstreamMs {
				return fmt.Errorf("value must be between %d and %d", minSlowUpstreamMs, maxSlowUpstreamMs)
			}
			return nil
		},
		Title:       "Slow upstream threshold (ms)",
		Description: "Response time at the 95th percentile above which a domain page warns that the app's upstream is slow.",
	})
}

// trafficSlowUpstream is the hint the domain page shows when an app answers slowly.
type trafficSlowUpstream struct {
	P95Ms       float64        `json:"p95_ms"`
	ThresholdMs int            `json:"threshold_ms"`
	Routes      []trafficRoute `json:"routes"`
}

func (rt *Router) slowUpstreamThreshold(ctx context.Context) int {
	n := defaultSlowUpstreamMs
	if rt.runtimeSettings != nil {
		if v := rt.runtimeSettings.Int(ctx, KeyTrafficSlowUpstreamMs); v > 0 {
			n = v
		}
	}
	return min(max(n, minSlowUpstreamMs), maxSlowUpstreamMs)
}

// slowUpstreamHint returns nil unless enough requests were seen and p95 passes the threshold.
func slowUpstreamHint(requests, p95Ms float64, thresholdMs int, slow []trafficRoute) *trafficSlowUpstream {
	if requests < slowUpstreamMinRequests || p95Ms < float64(thresholdMs) {
		return nil
	}
	routes := slow
	if len(routes) > slowUpstreamRouteCount {
		routes = routes[:slowUpstreamRouteCount]
	}
	return &trafficSlowUpstream{P95Ms: p95Ms, ThresholdMs: thresholdMs, Routes: routes}
}
