package api

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	// EnvTrafficSlowRouteMin is the fewest requests a route needs in the
	// window before it can rank among the slowest routes.
	EnvTrafficSlowRouteMin = "APP_TRAFFIC_SLOW_ROUTE_MIN"

	defaultTrafficSlowRouteMin = 5
	trafficTopRoutes           = 10
	trafficSampleLimit         = 50
)

var trafficWindows = map[string]time.Duration{
	"5m":  5 * time.Minute,
	"1h":  time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
}

// trafficLabel is one referrer host or country with its request count and
// share of the audience kind's total.
type trafficLabel struct {
	Label    string  `json:"label"`
	Requests float64 `json:"requests"`
	Share    float64 `json:"share"`
}

const trafficTopLabels = 10

func audienceLabels(rows []telemetry.AudienceRow, kind string) []trafficLabel {
	var total float64
	for _, r := range rows {
		if r.Kind == kind {
			total += r.Requests
		}
	}
	out := []trafficLabel{}
	for _, r := range rows {
		if r.Kind != kind || len(out) >= trafficTopLabels {
			continue
		}
		out = append(out, trafficLabel{Label: r.Label, Requests: r.Requests, Share: ratio(r.Requests, total)})
	}
	return out
}

type trafficStatusClasses struct {
	Status2xx float64 `json:"2xx"`
	Status3xx float64 `json:"3xx"`
	Status4xx float64 `json:"4xx"`
	Status5xx float64 `json:"5xx"`
}

type trafficLatency struct {
	P50Ms float64 `json:"p50_ms"`
	P95Ms float64 `json:"p95_ms"`
	P99Ms float64 `json:"p99_ms"`
}

type trafficReplica struct {
	Upstream       string  `json:"upstream"`
	Requests       float64 `json:"requests"`
	RatePerSec     float64 `json:"rate_per_sec"`
	ErrorRate4xx   float64 `json:"error_rate_4xx"`
	ErrorRate5xx   float64 `json:"error_rate_5xx"`
	UpstreamErrors float64 `json:"upstream_errors"`
	trafficLatency
}

type trafficRoute struct {
	Route        string  `json:"route"`
	Kind         string  `json:"kind"`
	Requests     float64 `json:"requests"`
	ErrorRate4xx float64 `json:"error_rate_4xx"`
	ErrorRate5xx float64 `json:"error_rate_5xx"`
	AvgMs        float64 `json:"avg_ms"`
}

type trafficSLO struct {
	Objective       string  `json:"objective"`
	Target          float64 `json:"target"`
	LatencyMs       float64 `json:"latency_ms,omitempty"`
	BurnRate        float64 `json:"burn_rate"`
	BudgetRemaining float64 `json:"budget_remaining"`
}

type trafficResponse struct {
	App            string                      `json:"app"`
	Window         string                      `json:"window"`
	From           time.Time                   `json:"from"`
	To             time.Time                   `json:"to"`
	HasTraffic     bool                        `json:"has_traffic"`
	Requests       float64                     `json:"requests"`
	RatePerSec     float64                     `json:"rate_per_sec"`
	Statuses       trafficStatusClasses        `json:"status_classes"`
	ErrorRate4xx   float64                     `json:"error_rate_4xx"`
	ErrorRate5xx   float64                     `json:"error_rate_5xx"`
	UpstreamErrors float64                     `json:"upstream_errors"`
	Latency        trafficLatency              `json:"latency"`
	Replicas       []trafficReplica            `json:"replicas"`
	SlowRoutes     []trafficRoute              `json:"slow_routes"`
	TopRoutes      []trafficRoute              `json:"top_routes"`
	KindTotals     map[string]trafficKindTotal `json:"kind_totals"`
	Samples        []telemetry.RequestSample   `json:"samples"`
	Referrers      []trafficLabel              `json:"referrers"`
	Countries      []trafficLabel              `json:"countries"`
	SLO            *trafficSLO                 `json:"slo,omitempty"`
	Shield         trafficShield               `json:"shield"`
	Abuse          trafficAbuse                `json:"abuse"`
	ErrorSpikes    []telemetry.ErrorSpike      `json:"error_spikes"`
}

func trafficSlowRouteMin() float64 {
	if n, err := strconv.Atoi(os.Getenv(EnvTrafficSlowRouteMin)); err == nil && n > 0 {
		return float64(n)
	}
	return defaultTrafficSlowRouteMin
}

func ratio(n, d float64) float64 {
	if d <= 0 {
		return 0
	}
	return n / d
}

func replicaRows(rows []telemetry.UpstreamRow, window time.Duration) []trafficReplica {
	out := make([]trafficReplica, 0, len(rows))
	for _, r := range rows {
		out = append(out, trafficReplica{
			Upstream:       r.Upstream,
			Requests:       r.Requests,
			RatePerSec:     r.Requests / window.Seconds(),
			ErrorRate4xx:   ratio(r.Errors4xx, r.Requests),
			ErrorRate5xx:   ratio(r.Errors5xx, r.Requests),
			UpstreamErrors: r.UpstreamErrors,
			trafficLatency: trafficLatency{
				P50Ms: telemetry.PercentileFromBuckets(r.Latency, 0.50),
				P95Ms: telemetry.PercentileFromBuckets(r.Latency, 0.95),
				P99Ms: telemetry.PercentileFromBuckets(r.Latency, 0.99),
			},
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Requests != out[j].Requests {
			return out[i].Requests > out[j].Requests
		}
		return out[i].Upstream < out[j].Upstream
	})
	return out
}

func slowRoutes(b telemetry.Breakdown, minRequests float64, n int) []trafficRoute {
	rows := make([]trafficRoute, 0, len(b.Routes))
	for _, r := range b.Routes {
		if r.Requests < minRequests || r.Route == telemetry.OverflowRoute {
			continue
		}
		kind := classifyRoute(r.Route)
		if kind == routeKindAsset {
			continue
		}
		rows = append(rows, trafficRoute{
			Route:        r.Route,
			Kind:         kind,
			Requests:     r.Requests,
			ErrorRate4xx: ratio(r.Errors4xx, r.Requests),
			ErrorRate5xx: ratio(r.Errors5xx, r.Requests),
			AvgMs:        r.LatencyMSSum / r.Requests,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].AvgMs != rows[j].AvgMs {
			return rows[i].AvgMs > rows[j].AvgMs
		}
		return rows[i].Route < rows[j].Route
	})
	if len(rows) > n {
		rows = rows[:n]
	}
	return rows
}

// topRoutes returns the n busiest routes of each kind, so a flood of static
// files cannot push the pages out of the list.
func topRoutes(b telemetry.Breakdown, n int) []trafficRoute {
	byKind := map[string][]trafficRoute{}
	for _, r := range b.Routes {
		if r.Requests <= 0 || r.Route == telemetry.OverflowRoute {
			continue
		}
		kind := classifyRoute(r.Route)
		byKind[kind] = append(byKind[kind], trafficRoute{
			Route:        r.Route,
			Kind:         kind,
			Requests:     r.Requests,
			ErrorRate4xx: ratio(r.Errors4xx, r.Requests),
			ErrorRate5xx: ratio(r.Errors5xx, r.Requests),
			AvgMs:        r.LatencyMSSum / r.Requests,
		})
	}
	var rows []trafficRoute
	for _, list := range byKind {
		sort.Slice(list, func(i, j int) bool {
			if list[i].Requests != list[j].Requests {
				return list[i].Requests > list[j].Requests
			}
			return list[i].Route < list[j].Route
		})
		if len(list) > n {
			list = list[:n]
		}
		rows = append(rows, list...)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Requests != rows[j].Requests {
			return rows[i].Requests > rows[j].Requests
		}
		return rows[i].Route < rows[j].Route
	})
	return rows
}

type trafficKindTotal struct {
	Requests     float64 `json:"requests"`
	ErrorRate5xx float64 `json:"error_rate_5xx"`
}

// kindTotals sums every route of a kind, not only the listed top ones.
func kindTotals(b telemetry.Breakdown) map[string]trafficKindTotal {
	reqs, errs := map[string]float64{}, map[string]float64{}
	for _, r := range b.Routes {
		kind := classifyRoute(r.Route)
		reqs[kind] += r.Requests
		errs[kind] += r.Errors5xx
	}
	out := make(map[string]trafficKindTotal, len(reqs))
	for kind, n := range reqs {
		out[kind] = trafficKindTotal{Requests: n, ErrorRate5xx: ratio(errs[kind], n)}
	}
	return out
}

func sumMetric(ctx context.Context, q telemetry.MetricsQuerier, app, metric string, from, to time.Time) float64 {
	samples, err := q.QueryMetrics(ctx, "service:"+app, metric, from, to)
	if err != nil && len(samples) == 0 {
		return 0
	}
	var t float64
	for _, s := range samples {
		if s.Value > 0 {
			t += s.Value
		}
	}
	return t
}

func (rt *Router) trafficSLO(ctx context.Context, app string, from, to time.Time) *trafficSLO {
	if rt.alertRules == nil {
		return nil
	}
	rules, err := rt.alertRules.ListRulesForResource(ctx, resourceIDForApp(app))
	if err != nil {
		rt.logger.Warn("api: traffic: slo rules unavailable", slog.String("error", err.Error()), slog.String("name", app))
		return nil
	}
	for _, rule := range rules {
		if rule.Kind != alerting.KindSLOBurn || rule.SLO == nil || !rule.Enabled {
			continue
		}
		counts, err := alerting.CountRequests(ctx, rt.telemetry, app, from, to, *rule.SLO)
		if err != nil {
			rt.logger.Warn("api: traffic: slo counts unavailable", slog.String("error", err.Error()), slog.String("name", app))
			return nil
		}
		return &trafficSLO{
			Objective:       rule.SLO.Objective,
			Target:          rule.SLO.Target,
			LatencyMs:       rule.SLO.LatencyMs,
			BurnRate:        alerting.BurnRate(counts.Bad, counts.Total, rule.SLO.Target),
			BudgetRemaining: alerting.BudgetRemaining(counts.Bad, counts.Total, rule.SLO.Target),
		}
	}
	return nil
}

// handleAppTraffic handles GET /api/v1/apps/{name}/traffic: RED totals,
// per-replica breakdown, slowest route templates, recent slow or failed
// request samples and the SLO error-budget burn, over window=5m|1h|24h.
func (rt *Router) handleAppTraffic(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	if _, found, err := rt.lookupAppResource(r.Context(), name); err != nil {
		rt.internalError(w, "api: traffic: load app", err)
		return
	} else if !found {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	label := r.URL.Query().Get("window")
	if label == "" {
		label = "5m"
	}
	window, ok := trafficWindows[label]
	if !ok {
		writeError(w, http.StatusBadRequest, "window must be 5m, 1h, 24h or 7d")
		return
	}
	ctx := r.Context()
	to := time.Now().UTC()
	from := to.Add(-window)
	resp := trafficResponse{
		App: name, Window: label, From: from, To: to,
		Replicas: []trafficReplica{}, SlowRoutes: []trafficRoute{}, TopRoutes: []trafficRoute{}, KindTotals: map[string]trafficKindTotal{}, Samples: []telemetry.RequestSample{},
		Referrers: []trafficLabel{}, Countries: []trafficLabel{}, ErrorSpikes: []telemetry.ErrorSpike{},
	}

	points, err := telemetry.QueryRequests(ctx, rt.telemetry, name, from, to, window+time.Second)
	if err != nil && points == nil {
		rt.internalError(w, "api: traffic: query requests", err)
		return
	}
	if len(points) > 0 {
		p := points[0]
		resp.HasTraffic = true
		resp.Requests, resp.RatePerSec = p.Requests, p.RatePerSec
		resp.ErrorRate4xx, resp.ErrorRate5xx = p.ErrorRate4xx, p.ErrorRate5xx
		resp.UpstreamErrors = p.UpstreamErrors
		resp.Latency = trafficLatency{P50Ms: p.P50Ms, P95Ms: p.P95Ms, P99Ms: p.P99Ms}
		resp.Statuses = trafficStatusClasses{
			Status2xx: sumMetric(ctx, rt.telemetry, name, telemetry.MetricHTTPResponses2xx, from, to),
			Status3xx: sumMetric(ctx, rt.telemetry, name, telemetry.MetricHTTPResponses3xx, from, to),
			Status4xx: sumMetric(ctx, rt.telemetry, name, telemetry.MetricHTTPResponses4xx, from, to),
			Status5xx: sumMetric(ctx, rt.telemetry, name, telemetry.MetricHTTPResponses5xx, from, to),
		}
	}

	rid := resourceIDForApp(name)
	if src, ok := rt.telemetry.(telemetry.BreakdownSource); ok {
		if b, berr := src.QueryBreakdown(ctx, rid, from, to); berr == nil {
			resp.SlowRoutes = slowRoutes(b, trafficSlowRouteMin(), trafficTopRoutes)
			resp.TopRoutes = topRoutes(b, trafficTopRoutes)
			resp.KindTotals = kindTotals(b)
		}
	}
	if src, ok := rt.telemetry.(telemetry.TrafficDetailSource); ok {
		if rows, uerr := src.QueryUpstreams(ctx, rid, from, to); uerr == nil {
			resp.Replicas = replicaRows(rows, window)
		}
		if samples, serr := src.QueryRequestSamples(ctx, rid, from, to, trafficSampleLimit); serr == nil && samples != nil {
			resp.Samples = samples
		}
	}
	if src, ok := rt.telemetry.(telemetry.AudienceSource); ok {
		if rows, aerr := src.QueryAudience(ctx, rid, from, to); aerr == nil {
			resp.Referrers = audienceLabels(rows, telemetry.AudienceReferrer)
			resp.Countries = audienceLabels(rows, telemetry.AudienceCountry)
		}
	}
	resp.ErrorSpikes = rt.errorSpikes(ctx, name, from, to, window)
	resp.SLO = rt.trafficSLO(ctx, name, from, to)
	resp.Shield = shieldCounts(ctx, rt.telemetry, name, from, to)
	resp.Abuse = abuseTraffic(ctx, rt.telemetry, name, from, to)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, resp)
}

// errorSpikes buckets the window, finds 5xx runs that stand out and ties each
// to the deploy that most plausibly caused it.
func (rt *Router) errorSpikes(ctx context.Context, app string, from, to time.Time, window time.Duration) []telemetry.ErrorSpike {
	cfg := telemetry.SpikeConfigFromEnv()
	step := window / time.Duration(cfg.Buckets)
	if step < telemetry.MinRequestStep {
		step = telemetry.MinRequestStep
	}
	points, err := telemetry.QueryRequests(ctx, rt.telemetry, app, from, to, step)
	if err != nil && points == nil {
		rt.logger.Warn("api: traffic: spike series unavailable", slog.String("error", err.Error()), slog.String("name", app))
		return []telemetry.ErrorSpike{}
	}
	var markers []telemetry.DeployMarker
	if rt.deployAttempts != nil {
		attempts, aerr := rt.deployAttempts.ListDeployAttempts(ctx, app)
		if aerr != nil {
			rt.logger.Warn("api: traffic: deploy markers unavailable", slog.String("error", aerr.Error()), slog.String("name", app))
		}
		for _, a := range attempts {
			markers = append(markers, telemetry.DeployMarker{ID: a.ID, At: a.StartedAt, Status: a.Status, Image: a.Image, Commit: a.CommitSHA})
		}
	}
	return telemetry.DetectErrorSpikes(points, step, markers, cfg)
}
