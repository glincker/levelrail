package api

import (
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	batchMetricsWindow     = time.Hour
	batchMetricsStep       = 5 * time.Minute
	defaultBatchMetricsMax = 200
)

func batchMetricsMax() int {
	if v, err := strconv.Atoi(os.Getenv("APP_APPS_METRICS_MAX")); err == nil && v > 0 {
		return v
	}
	return defaultBatchMetricsMax
}

// appMetricsSummary is one app's row in GET /api/v1/apps-metrics.
type appMetricsSummary struct {
	Name         string     `json:"name"`
	CPUPercent   *float64   `json:"cpu_percent,omitempty"`
	MemoryBytes  *float64   `json:"memory_usage_bytes,omitempty"`
	MemoryLimit  *float64   `json:"memory_limit_bytes,omitempty"`
	HasTraffic   bool       `json:"has_traffic"`
	RatePerSec   float64    `json:"rate_per_sec"`
	ErrorRate5xx float64    `json:"error_rate_5xx"`
	P95Ms        float64    `json:"p95_ms"`
	Spark        []float64  `json:"spark"`
	LastDeployAt *time.Time `json:"last_deploy_at,omitempty"`
}

// handleBatchAppMetrics handles GET /api/v1/apps-metrics: latest resource
// usage plus a one hour request summary and sparkline for every readable app
// (or the comma separated names query param), in one response.
func (rt *Router) handleBatchAppMetrics(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}
	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: apps metrics: visibility", err)
		return
	}
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.internalError(w, "api: apps metrics: list apps", err)
		return
	}
	wanted := map[string]bool{}
	for _, n := range strings.Split(r.URL.Query().Get("names"), ",") {
		if n = strings.TrimSpace(n); n != "" {
			wanted[n] = true
		}
	}
	names := make([]string, 0, len(svcs))
	for _, s := range svcs {
		if canSee(s.Name) && (len(wanted) == 0 || wanted[s.Name]) {
			names = append(names, s.Name)
		}
	}
	sort.Strings(names)
	if limit := batchMetricsMax(); len(names) > limit {
		names = names[:limit]
	}

	byName := make(map[string]*appMetricsSummary, len(names))
	for _, n := range names {
		byName[n] = &appMetricsSummary{Name: n, Spark: make([]float64, int(batchMetricsWindow/batchMetricsStep))}
	}
	rt.fillLatestUsage(r, byName)

	now := time.Now()
	from := now.Add(-batchMetricsWindow)
	for _, n := range names {
		rt.fillRequestSummary(r, byName[n], from, now)
	}

	out := make([]appMetricsSummary, 0, len(names))
	for _, n := range names {
		out = append(out, *byName[n])
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) fillRequestSummary(r *http.Request, row *appMetricsSummary, from, now time.Time) {
	sum, err := telemetry.SummarizeRequests(r.Context(), rt.telemetry, row.Name, batchMetricsWindow, now)
	if err != nil && !sum.HasTraffic {
		rt.logger.Warn("api: apps metrics: summary unavailable", slog.String("app", row.Name), slog.String("error", err.Error()))
	} else {
		row.HasTraffic = sum.HasTraffic
		row.RatePerSec = sum.RatePerSec
		row.ErrorRate5xx = sum.ErrorRate5xx
		row.P95Ms = sum.P95Ms
	}
	points, _ := telemetry.QueryRequests(r.Context(), rt.telemetry, row.Name, from, now, batchMetricsStep)
	for _, p := range points {
		if i := int(p.Timestamp.Sub(from) / batchMetricsStep); i >= 0 && i < len(row.Spark) {
			row.Spark[i] = p.RatePerSec
		}
	}
	if rt.deployAttempts == nil {
		return
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(r.Context(), row.Name)
	if err != nil || len(attempts) == 0 {
		return
	}
	t := attempts[0].StartedAt
	if attempts[0].FinishedAt != nil {
		t = *attempts[0].FinishedAt
	}
	row.LastDeployAt = &t
}

func (rt *Router) fillLatestUsage(r *http.Request, byName map[string]*appMetricsSummary) {
	for _, metric := range []string{"cpu_percent", "memory_usage_bytes", "memory_limit_bytes"} {
		samples, err := rt.telemetry.LatestByMetric(r.Context(), metric)
		if err != nil {
			rt.logger.Warn("api: apps metrics: latest failed", slog.String("metric", metric), slog.String("error", err.Error()))
			continue
		}
		for _, s := range samples {
			name, ok := strings.CutPrefix(s.ResourceID, "service:")
			if !ok || byName[name] == nil {
				continue
			}
			v := s.Value
			switch metric {
			case "cpu_percent":
				byName[name].CPUPercent = &v
			case "memory_usage_bytes":
				byName[name].MemoryBytes = &v
			case "memory_limit_bytes":
				byName[name].MemoryLimit = &v
			}
		}
	}
}
