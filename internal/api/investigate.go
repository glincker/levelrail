package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/investigate"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Investigate env vars and defaults.
const (
	envInvestigateLookback = "APP_INVESTIGATE_LOOKBACK"
	envInvestigateMaxSpan  = "APP_INVESTIGATE_MAX_SPAN"

	defaultInvestigateLookback = 30 * time.Minute
	defaultInvestigateMaxSpan  = 24 * time.Hour
	investigateTopRoutes       = 10
	investigateMaxChanges      = 200
	investigateRestartGap      = 2 * time.Minute

	metricCPU         = "cpu_percent"
	metricMemoryUsage = "memory_usage_bytes"
	metricMemoryLimit = "memory_limit_bytes"
)

type investigateSummary struct {
	HasTraffic   bool    `json:"has_traffic"`
	Requests     float64 `json:"requests"`
	RatePerSec   float64 `json:"rate_per_sec"`
	ErrorRate4xx float64 `json:"error_rate_4xx"`
	ErrorRate5xx float64 `json:"error_rate_5xx"`
	P50Ms        float64 `json:"p50_ms"`
	P95Ms        float64 `json:"p95_ms"`
	P99Ms        float64 `json:"p99_ms"`
}

type investigateRoute struct {
	Route        string  `json:"route"`
	Host         string  `json:"host,omitempty"`
	Requests     float64 `json:"requests"`
	Share        float64 `json:"share"`
	ErrorRate4xx float64 `json:"error_rate_4xx"`
	ErrorRate5xx float64 `json:"error_rate_5xx"`
	AvgMs        float64 `json:"avg_ms"`
}

type investigateStatus struct {
	Status int     `json:"status"`
	Count  float64 `json:"count"`
	Share  float64 `json:"share"`
}

type investigateLogs struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type investigateResponse struct {
	App             string              `json:"app"`
	From            time.Time           `json:"from"`
	To              time.Time           `json:"to"`
	StepSeconds     float64             `json:"step_seconds"`
	Summary         investigateSummary  `json:"summary"`
	Baseline        investigateSummary  `json:"baseline"`
	RoutesAvailable bool                `json:"routes_available"`
	TopRoutes       []investigateRoute  `json:"top_routes"`
	StatusCodes     []investigateStatus `json:"status_codes"`
	Timeline        []investigate.Event `json:"timeline"`
	Logs            investigateLogs     `json:"logs"`
}

// windowSummary summarises [from, to] as one bucket so percentiles come from
// the merged latency histogram rather than an average of step percentiles.
func (rt *Router) windowSummary(r *http.Request, app string, from, to time.Time) investigateSummary {
	span := to.Sub(from)
	pts, err := telemetry.QueryRequests(r.Context(), rt.telemetry, app, from, to, span+time.Second)
	if err != nil && pts == nil {
		rt.logger.Warn("api: investigate: request summary unavailable", slog.String("error", err.Error()), slog.String("name", app))
		return investigateSummary{}
	}
	if len(pts) == 0 {
		return investigateSummary{}
	}
	p := pts[0]
	return investigateSummary{
		HasTraffic: true, Requests: p.Requests, RatePerSec: p.RatePerSec,
		ErrorRate4xx: p.ErrorRate4xx, ErrorRate5xx: p.ErrorRate5xx,
		P50Ms: p.P50Ms, P95Ms: p.P95Ms, P99Ms: p.P99Ms,
	}
}

// handleInvestigate handles GET /api/v1/apps/{name}/investigate: everything
// that explains a spike in [from, to] in one response, namely the request
// summary against the preceding window, the busiest routes and status codes,
// and one merged timeline of deploys, config changes, restarts and saturation.
func (rt *Router) handleInvestigate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(ctx, name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: investigate: load app failed", err, slog.String("name", name))
		return
	}
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}
	from, to, err := parseTimeRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	span := to.Sub(from)
	if span <= 0 {
		writeError(w, http.StatusBadRequest, "to must be after from")
		return
	}
	if limit := envDuration(envInvestigateMaxSpan, defaultInvestigateMaxSpan); span > limit {
		writeError(w, http.StatusBadRequest, "window is wider than "+limit.String()+"; narrow it to the spike")
		return
	}

	lookback := envDuration(envInvestigateLookback, defaultInvestigateLookback)
	histFrom := from.Add(-lookback)
	rid := resourceIDForApp(name)
	resp := investigateResponse{
		App: name, From: from, To: to, StepSeconds: span.Seconds(),
		TopRoutes: []investigateRoute{}, StatusCodes: []investigateStatus{},
		Logs: investigateLogs{From: from, To: to},
	}

	resp.Summary = rt.windowSummary(r, name, from, to)
	resp.Baseline = rt.windowSummary(r, name, from.Add(-span), from)

	if fed, ok := rt.telemetry.(interface {
		QueryBreakdown(ctx context.Context, resourceID string, from, to time.Time) (telemetry.Breakdown, error)
	}); ok {
		resp.RoutesAvailable = telemetry.RoutesEnabled()
		bd, berr := fed.QueryBreakdown(ctx, rid, from, to)
		if berr != nil && len(bd.Routes) == 0 && len(bd.Statuses) == 0 {
			rt.logger.Warn("api: investigate: breakdown unavailable", slog.String("error", berr.Error()), slog.String("name", name))
		}
		total := bd.Total()
		for _, rr := range bd.TopRoutes(investigateTopRoutes) {
			row := investigateRoute{Route: rr.Route, Requests: rr.Requests}
			if rr.Requests > 0 {
				row.ErrorRate4xx = rr.Errors4xx / rr.Requests
				row.ErrorRate5xx = rr.Errors5xx / rr.Requests
				row.AvgMs = rr.LatencyMSSum / rr.Requests
			}
			if total > 0 {
				row.Share = rr.Requests / total
			}
			resp.TopRoutes = append(resp.TopRoutes, row)
		}
		for _, sr := range bd.SortedStatuses() {
			row := investigateStatus{Status: sr.Status, Count: sr.Count}
			if total > 0 {
				row.Share = sr.Count / total
			}
			resp.StatusCodes = append(resp.StatusCodes, row)
		}
	}

	resp.Timeline = rt.investigateTimeline(r, name, rid, histFrom, to)
	writeJSON(w, http.StatusOK, resp)
}

func (rt *Router) investigateTimeline(r *http.Request, name, rid string, histFrom, to time.Time) []investigate.Event {
	ctx := r.Context()
	agg := rt.changeAggregator()
	agg.Window = to.Sub(histFrom)
	agg.Max = investigateMaxChanges
	chg := agg.Collect(ctx, name, to)

	query := func(metric string) []telemetry.Sample {
		s, err := rt.telemetry.QueryMetrics(ctx, rid, metric, histFrom, to)
		if err != nil && len(s) == 0 {
			rt.logger.Warn("api: investigate: metric query failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("metric", metric))
		}
		return s
	}
	restarts := query(telemetry.MetricContainerRestartCount)
	th := investigate.ThresholdsFromEnv()
	sat := investigate.SaturationEvents(query(metricCPU), query(metricMemoryUsage), query(metricMemoryLimit), th)

	merged := investigate.Merge(
		investigate.FromChanges(chg.Changes),
		investigate.RestartEvents(restarts, investigateRestartGap),
		sat,
	)
	return investigate.Window(merged, histFrom, to)
}
