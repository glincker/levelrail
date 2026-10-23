package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	canSee, err := rt.scopedAppVisibility(r)
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

	overviews := rt.requestOverviews(r.Context(), names)
	from := time.Now().Add(-batchMetricsWindow)
	var lastDeploy map[string]time.Time
	batched := false
	if src, ok := rt.deployAttempts.(latestDeployActivitySource); ok {
		done := timePhase(r.Context(), "deploy_activity")
		lastDeploy, err = src.LatestDeployActivity(r.Context())
		done()
		batched = err == nil
		if err != nil {
			rt.logger.Warn("api: apps metrics: last deploy times unavailable", slog.String("error", err.Error()))
		}
	}
	for _, n := range names {
		row := byName[n]
		ov := overviews[n]
		row.HasTraffic = ov.Summary.HasTraffic
		row.RatePerSec = ov.Summary.RatePerSec
		row.ErrorRate5xx = ov.Summary.ErrorRate5xx
		row.P95Ms = ov.Summary.P95Ms
		for _, p := range ov.Points {
			if i := int(p.Timestamp.Sub(from) / batchMetricsStep); i >= 0 && i < len(row.Spark) {
				row.Spark[i] = p.RatePerSec
			}
		}
		if t, ok := lastDeploy[n]; ok {
			row.LastDeployAt = &t
		} else if !batched {
			rt.fillLastDeploy(r, row)
		}
	}

	out := make([]appMetricsSummary, 0, len(names))
	for _, n := range names {
		out = append(out, *byName[n])
	}
	writeJSON(w, http.StatusOK, out)
}

// latestDeployActivitySource is the optional batch read of every service's
// newest deploy attempt time, one query instead of one list per app.
type latestDeployActivitySource interface {
	LatestDeployActivity(ctx context.Context) (map[string]time.Time, error)
}

// requestOverviews reads the one hour request summaries for names through
// the dashboard cache. Keyed by the exact name set, so a restricted viewer
// can only ever hit entries built for the same visible apps.
func (rt *Router) requestOverviews(ctx context.Context, names []string) map[string]telemetry.RequestOverview {
	defer timePhase(ctx, "telemetry_requests")()
	load := func(ctx context.Context) (any, error) {
		ov, err := telemetry.RequestOverviews(ctx, rt.telemetry, names, batchMetricsWindow, batchMetricsStep, time.Now())
		if err != nil && len(ov) == 0 {
			rt.logger.Warn("api: apps metrics: summaries unavailable", slog.String("error", err.Error()))
		}
		return ov, nil
	}
	if rt.dash.ttl <= 0 {
		v, _ := load(ctx)
		ov, _ := v.(map[string]telemetry.RequestOverview)
		return ov
	}
	sum := sha256.Sum256([]byte(strings.Join(names, "\x00")))
	v, err := rt.dash.get(ctx, "overview:"+hex.EncodeToString(sum[:]), load)
	if err != nil {
		return nil
	}
	ov, _ := v.(map[string]telemetry.RequestOverview)
	return ov
}

func (rt *Router) fillLastDeploy(r *http.Request, row *appMetricsSummary) {
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
		samples, err := rt.latestByMetric(r.Context(), metric)
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
