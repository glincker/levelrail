package api

import (
	"context"
	"log/slog"
	"strings"
)

const percentPerCore = 100

// quotaServicesSource lists the apps in a quota scope; *store.DB satisfies it.
type quotaServicesSource interface {
	QuotaScopeServices(ctx context.Context, scope, scopeID string) ([]string, error)
}

// quotaLiveResource is what a scope's apps actually use right now, from the
// latest metrics samples, to sit next to the reserved amounts.
type quotaLiveResource struct {
	MemoryMB    int64   `json:"memory_mb"`
	CPU         float64 `json:"cpu"`
	SampledApps int     `json:"sampled_apps"`
	TotalApps   int     `json:"total_apps"`
}

// quotaLiveUsage sums the newest CPU and memory sample of every app in the
// scope. It returns nil when telemetry or the scope listing is unavailable,
// so the response simply omits live usage.
func (rt *Router) quotaLiveUsage(ctx context.Context, db quotaStore, scope, scopeID string) *quotaLiveResource {
	src, ok := db.(quotaServicesSource)
	if !ok || rt.telemetry == nil {
		return nil
	}
	names, err := src.QuotaScopeServices(ctx, scope, scopeID)
	if err != nil {
		rt.logger.Warn("api: quota: list scope apps failed", slog.String("error", err.Error()))
		return nil
	}
	inScope := make(map[string]bool, len(names))
	for _, n := range names {
		inScope[n] = true
	}
	live := &quotaLiveResource{TotalApps: len(names)}
	var memBytes, cpuPercent float64
	sampled := map[string]bool{}
	for _, metric := range []string{"cpu_percent", "memory_usage_bytes"} {
		samples, err := rt.latestByMetric(ctx, metric)
		if err != nil {
			rt.logger.Warn("api: quota: live usage metric failed", slog.String("metric", metric), slog.String("error", err.Error()))
			continue
		}
		for _, s := range samples {
			name, ok := strings.CutPrefix(s.ResourceID, "service:")
			if !ok || !inScope[name] {
				continue
			}
			sampled[name] = true
			if metric == "cpu_percent" {
				cpuPercent += s.Value
			} else {
				memBytes += s.Value
			}
		}
	}
	live.SampledApps = len(sampled)
	live.MemoryMB = int64(memBytes) / bytesPerMB
	live.CPU = cpuPercent / percentPerCore
	return live
}
