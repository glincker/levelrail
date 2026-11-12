package api

import (
	"context"
	"fmt"
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
	"disk_read_bytes",
	"disk_write_bytes",
}

// Fields are omitted, not zero, when telemetry has never recorded that metric for the app.
type appResourceUsageResource struct {
	Name             string   `json:"name"`
	CPUPercent       *float64 `json:"cpu_percent,omitempty"`
	MemoryUsageBytes *float64 `json:"memory_usage_bytes,omitempty"`
	MemoryLimitBytes *float64 `json:"memory_limit_bytes,omitempty"`
	NetworkRxBytes   *float64 `json:"network_rx_bytes,omitempty"`
	NetworkTxBytes   *float64 `json:"network_tx_bytes,omitempty"`
	DiskReadBytes    *float64 `json:"disk_read_bytes,omitempty"`
	DiskWriteBytes   *float64 `json:"disk_write_bytes,omitempty"`
}

// handleAppResourceUsage returns every app in one call, so the dashboard avoids per-app fetches.
// Apps with no samples yet are included with absent fields, not dropped.
func (rt *Router) handleAppResourceUsage(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}
	// Keyed per router so routers sharing a process never serve each other's cached list.
	all, err := rt.expensiveRead(r.Context(), fmt.Sprintf("app-resource-usage:%p", rt), func(ctx context.Context) (any, error) {
		return rt.computeAppResourceUsage(ctx)
	})
	if err != nil {
		rt.internalError(w, "api: app resource usage: list apps failed", err)
		return
	}
	canSee, err := rt.scopedAppVisibility(r)
	if err != nil {
		rt.internalError(w, "api: app resource usage: visibility", err)
		return
	}
	out := []appResourceUsageResource{}
	for _, u := range all.([]appResourceUsageResource) {
		if canSee(u.Name) {
			out = append(out, u)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// computeAppResourceUsage is unscoped so one cached result serves every caller, who then filter by visibility.
func (rt *Router) computeAppResourceUsage(ctx context.Context) ([]appResourceUsageResource, error) {
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, err
	}

	byName := make(map[string]*appResourceUsageResource, len(svcs))
	for _, svc := range svcs {
		byName[svc.Name] = &appResourceUsageResource{Name: svc.Name}
	}

	for _, metric := range resourceUsageMetrics {
		samples, err := rt.latestByMetric(ctx, metric)
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
			case "disk_read_bytes":
				usage.DiskReadBytes = &value
			case "disk_write_bytes":
				usage.DiskWriteBytes = &value
			}
		}
	}

	out := make([]appResourceUsageResource, 0, len(byName))
	for _, usage := range byName {
		out = append(out, *usage)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out, nil
}
