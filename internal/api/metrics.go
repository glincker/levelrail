package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// TelemetryQuerier is the surface the metrics and logs query handlers
// need. *telemetry.Federator satisfies this
// structurally: today it fans out to exactly one source (this node's
// own local store), the same single-node-now shape already established
// for the reconcile agent transport, so Phase 3's
// real multi-node federation slots in here without this package
// changing.
type TelemetryQuerier interface {
	QueryMetrics(ctx context.Context, resourceID, metric string, from, to time.Time) ([]telemetry.Sample, error)
	QueryLogs(ctx context.Context, resourceID string, from, to time.Time, query string) ([]telemetry.LogEntry, error)
	// LatestByMetric backs handleAppResourceUsage
	// (app_resource_usage.go): the dashboard-wide "what is every app
	// doing right now" ranking, one query per metric rather than one
	// query per app.
	LatestByMetric(ctx context.Context, metric string) ([]telemetry.Sample, error)
}

// resourceIDForApp is telemetry's stable identifier for one app's
// metrics/logs, matching cmd/levelrail/main.go's telemetryTargets,
// which is what actually writes samples under this key. A rename here
// without a matching rename there silently orphans every future query.
func resourceIDForApp(name string) string {
	return "service:" + name
}

// resourceLookup confirms name identifies an existing resource of some
// kind (app, database, ...) for telemetry purposes and resolves the
// resourceID it's stored under. found is false only when the resource
// genuinely doesn't exist, distinct from err which signals a real
// failure reaching the store; the shared query/stream handlers below
// turn that distinction into 404 vs 500.
type resourceLookup func(ctx context.Context, name string) (resourceID string, found bool, err error)

// lookupAppResource is the app-kind resourceLookup, backing
// handleQueryLogs, handleLiveLogStream, and handleQueryMetrics.
func (rt *Router) lookupAppResource(ctx context.Context, name string) (string, bool, error) {
	if _, err := rt.apps.GetDesiredService(ctx, name); errors.Is(err, store.ErrServiceNotFound) {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}
	return resourceIDForApp(name), true, nil
}

// defaultQueryWindow is the lookback applied when a caller omits `from`,
// a reasonable default for "what's this app doing right now" rather
// than requiring every dashboard request to compute and pass an
// explicit window.
const defaultQueryWindow = time.Hour

type metricPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Count     int       `json:"count"`
	// Max is the bucket's largest raw sample, so spikes survive averaging.
	Max float64 `json:"max"`
}

type metricsResponse struct {
	Metric string `json:"metric"`
	// StepSeconds is the bucket width used, 0 for raw samples.
	StepSeconds float64 `json:"step_seconds"`
	// Downsampled is true when max_points forced a coarser step than asked.
	Downsampled    bool          `json:"downsampled"`
	Points         []metricPoint `json:"points"`
	PreviousPoints []metricPoint `json:"previous_points,omitempty"`
}

// handleQueryMetrics handles GET /api/v1/apps/{name}/metrics; see
// queryResourceMetrics for the shared implementation.
func (rt *Router) handleQueryMetrics(w http.ResponseWriter, r *http.Request) {
	rt.queryResourceMetrics(w, r, rt.lookupAppResource, "query metrics", "app")
}

// queryResourceMetrics is the shared body behind handleQueryMetrics and
// handleQueryDatabaseMetrics (database_metrics.go): resolve name via
// lookup, then apply the same query params to both resource kinds
// (metric required; from/to RFC3339, default now-1h to now; step a Go
// duration string like "60s", omitted or "0" meaning raw unaggregated
// samples per telemetry.Aggregate's own step<=0 contract). opName and
// noun feed the log lines and 404 message so an app 404 reads
// "app not found" and a database 404 reads "database not found".
func (rt *Router) queryResourceMetrics(w http.ResponseWriter, r *http.Request, lookup resourceLookup, opName, noun string) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}

	name := r.PathValue("name")

	resourceID, found, err := lookup(r.Context(), name)
	if !found && err == nil {
		writeError(w, http.StatusNotFound, noun+" not found")
		return
	} else if err != nil {
		rt.logger.Error("api: "+opName+": load "+noun+" failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	metric := r.URL.Query().Get("metric")
	if metric == "" {
		writeError(w, http.StatusBadRequest, "metric query parameter is required")
		return
	}

	from, to, err := parseTimeRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	step, err := parseStep(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	samples, err := rt.telemetry.QueryMetrics(r.Context(), resourceID, metric, from, to)
	if err != nil {
		// A federated query with a partial result (ADR 008: some agents
		// unreachable) still has real data to return; only a total
		// failure (every source errored, samples empty and err set) is
		// treated as a request failure here. Federator.QueryMetrics
		// doesn't distinguish "partial" from "total" in its return
		// shape, so this checks the one signal available: whether
		// anything at all came back.
		if len(samples) == 0 {
			rt.logger.Error("api: "+opName+" failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("metric", metric))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		rt.logger.Warn("api: "+opName+": partial result", slog.String("error", err.Error()), slog.String("name", name), slog.String("metric", metric))
	}

	maxPoints, err := parseMaxPoints(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	step, downsampled := capStep(step, to.Sub(from), maxPoints)

	resp := metricsResponse{
		Metric:      metric,
		StepSeconds: step.Seconds(),
		Downsampled: downsampled,
		Points:      toMetricPoints(telemetry.Aggregate(samples, from, step)),
	}

	if r.URL.Query().Get("compare") == "previous" {
		span := to.Sub(from)
		prevFrom := from.Add(-span)
		prev, perr := rt.telemetry.QueryMetrics(r.Context(), resourceID, metric, prevFrom, from)
		if perr != nil && len(prev) == 0 {
			rt.logger.Warn("api: "+opName+": previous period unavailable", slog.String("error", perr.Error()), slog.String("name", name), slog.String("metric", metric))
		}
		shifted := telemetry.ShiftPoints(telemetry.Aggregate(prev, prevFrom, step), span)
		resp.PreviousPoints = toMetricPoints(shifted)
	}

	writeJSON(w, http.StatusOK, resp)
}

func toMetricPoints(agg []telemetry.AggregatedPoint) []metricPoint {
	points := make([]metricPoint, len(agg))
	for i, a := range agg {
		points[i] = metricPoint{Timestamp: a.Timestamp, Value: a.Value, Count: a.Count, Max: a.Max}
	}
	return points
}

// parseMaxPoints reads max_points; 0 means no cap. A request is clamped to
// APP_METRICS_MAX_POINTS so one call cannot ask for an unbounded series.
func parseMaxPoints(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("max_points")
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, errors.New("max_points must be a positive integer")
	}
	return min(n, telemetry.MaxPointsFromEnv()), nil
}

// capStep widens step so span holds at most maxPoints buckets. A zero step
// (raw samples) is only bucketed when raw would exceed maxPoints.
func capStep(step, span time.Duration, maxPoints int) (time.Duration, bool) {
	if maxPoints <= 0 || span <= 0 {
		return step, false
	}
	floor := step
	if floor <= 0 {
		floor = telemetry.MinRequestStep
	}
	if span/floor <= time.Duration(maxPoints) {
		return step, false
	}
	return telemetry.AutoStep(span, maxPoints, floor), true
}

// parseTimeRange reads from/to query params (RFC3339), defaulting to
// [now-defaultQueryWindow, now] when from is omitted; to defaults to now
// when omitted even if from is set, so "just give me everything since X"
// is a valid request without also requiring an explicit upper bound.
func parseTimeRange(r *http.Request) (from, to time.Time, err error) {
	now := time.Now()
	to = now
	from = now.Add(-defaultQueryWindow)

	if raw := r.URL.Query().Get("to"); raw != "" {
		to, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("to must be RFC3339")
		}
	}
	if raw := r.URL.Query().Get("from"); raw != "" {
		from, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("from must be RFC3339")
		}
	}
	if from.After(to) {
		return time.Time{}, time.Time{}, errors.New("from must not be after to")
	}
	return from, to, nil
}

func parseStep(r *http.Request) (time.Duration, error) {
	raw := r.URL.Query().Get("step")
	if raw == "" {
		return 0, nil
	}
	step, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errors.New("step must be a valid duration (e.g. \"60s\")")
	}
	return step, nil
}
