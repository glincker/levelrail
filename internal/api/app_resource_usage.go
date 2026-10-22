package api

import (
	"log/slog"
	"net/http"
	"sort"
	"strings"
)

// One LatestByMetric call per entry, each a single indexed query, not one per app.
var resourceUsageMetrics = []string{
	"cpu_percent",
	"memory_usage_bytes",
	"memory_limit_bytes",
	"network_rx_bytes",
	"network_tx_bytes",
}

// Fields are omitted, not zero, when telemetry has never recorded that metric for the app.
type appResourceUsageResource struct {
	Name             string   `json:"name"`
	CPUPercent       *float64 `json:"cpu_percent,omitempty"`
	MemoryUsageBytes *float64 `json:"memory_usage_bytes,omitempty"`
	MemoryLimitBytes *float64 `json:"memory_limit_bytes,omitempty"`
	NetworkRxBytes   *float64 `json:"network_rx_bytes,omitempty"`
	NetworkTxBytes   *float64 `json:"network_tx_bytes,omitempty"`
}

// handleAppResourceUsage returns every app in one call, so the dashboard avoids per-app fetches.
// Apps with no samples yet are included with absent fields, not dropped.
func (rt *Router) handleAppResourceUsage(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}

	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.internalError(w, "api: app resource usage: list apps failed", err)
		return
	}

	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: app resource usage: visibility", err)
		return
	}
	byName := make(map[string]*appResourceUsageResource, len(svcs))
	for _, svc := range svcs {
		if !canSee(svc.Name) {
			continue
		}
		byName[svc.Name] = &appResourceUsageResource{Name: svc.Name}
	}

	for _, metric := range resourceUsageMetrics {
		samples, err := rt.latestByMetric(r.Context(), metric)
		if err != nil {
			// A failing metric query must not blank out the metrics that succeeded.
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
