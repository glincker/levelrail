package api

import (
	"log/slog"
	"net/http"
	"sort"
	"strings"
)

// resourceUsageMetrics is every metric handleAppResourceUsage reads,
// the same permanent vocabulary sampleValues (internal/telemetry/
// collector.go) writes under. One LatestByMetric call per entry here,
// each a single indexed SQLite query, not one query per app.
var resourceUsageMetrics = []string{
	"cpu_percent",
	"memory_usage_bytes",
	"memory_limit_bytes",
	"network_rx_bytes",
	"network_tx_bytes",
}

// appResourceUsageResource is one app's latest-known resource footprint,
// the wire shape behind the dashboard's "what's using the most CPU/
// memory/traffic right now" ranking. Any field can be absent (omitted,
// not zero) when this control plane's telemetry has never recorded that
// metric for this app yet, e.g. right after a fresh deploy before the
// first collection tick.
type appResourceUsageResource struct {
	Name             string   `json:"name"`
	CPUPercent       *float64 `json:"cpu_percent,omitempty"`
	MemoryUsageBytes *float64 `json:"memory_usage_bytes,omitempty"`
	MemoryLimitBytes *float64 `json:"memory_limit_bytes,omitempty"`
	NetworkRxBytes   *float64 `json:"network_rx_bytes,omitempty"`
	NetworkTxBytes   *float64 `json:"network_tx_bytes,omitempty"`
}

// handleAppResourceUsage handles GET /api/v1/apps/resource-usage: the
// latest CPU/memory/network reading for every app in one response, so
// the dashboard can rank "which app is consuming the most resources
// right now" without fetching each app's own metrics one at a time (the
// N+1 shape section 4.12's "never fetch the full resource graph on page
// load" rule warns against). Returns every app that exists, including
// ones telemetry has no samples for yet (all fields absent), so a
// freshly deployed app is a real zero-usage row, not a missing one.
func (rt *Router) handleAppResourceUsage(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}

	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.logger.Error("api: app resource usage: list apps failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	byName := make(map[string]*appResourceUsageResource, len(svcs))
	for _, svc := range svcs {
		byName[svc.Name] = &appResourceUsageResource{Name: svc.Name}
	}

	for _, metric := range resourceUsageMetrics {
		samples, err := rt.telemetry.LatestByMetric(r.Context(), metric)
		if err != nil {
			// Partial telemetry (one metric's query failing) must not
			// blank out every other metric that did succeed: the same
			// "handle partial results gracefully" contract ADR 008
			// already requires of Federator.QueryMetrics.
			rt.logger.Warn("api: app resource usage: query metric failed", slog.String("metric", metric), slog.String("error", err.Error()))
			continue
		}
		for _, s := range samples {
			name, ok := strings.CutPrefix(s.ResourceID, "service:")
			if !ok {
				continue // not an app resource (e.g. a database's own resource_id)
			}
			usage, tracked := byName[name]
			if !tracked {
				continue // stale telemetry for an app that no longer exists
			}
			value := s.Value
			switch metric {
			case "cpu_percent":
				usage.CPUPercent = &value
			case "memory_usage_bytes":
				usage.MemoryUsageBytes = &value
			case "memory_limit_bytes":
				usage.MemoryLimitBytes = &value
			case "network_rx_bytes":
				usage.NetworkRxBytes = &value
			case "network_tx_bytes":
				usage.NetworkTxBytes = &value
			}
		}
	}

	out := make([]appResourceUsageResource, 0, len(byName))
	for _, usage := range byName {
		out = append(out, *usage)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	writeJSON(w, http.StatusOK, out)
}
