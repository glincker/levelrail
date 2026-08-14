package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// TelemetryQuerier is the surface the metrics and logs query handlers
// need (TASKS.md 2.3). *telemetry.Federator satisfies this
// structurally: today it fans out to exactly one source (this node's
// own local store), the same single-node-now shape already established
// for the reconcile agent transport, so Phase 3's
// real multi-node federation slots in here without this package
// changing.
type TelemetryQuerier interface {
	QueryMetrics(ctx context.Context, resourceID, metric string, from, to time.Time) ([]telemetry.Sample, error)
	QueryLogs(ctx context.Context, resourceID string, from, to time.Time, query string) ([]telemetry.LogEntry, error)
}

// resourceIDForApp is telemetry's stable identifier for one app's
// metrics/logs, matching cmd/levelrail/main.go's telemetryTargets,
// which is what actually writes samples under this key. A rename here
// without a matching rename there silently orphans every future query.
func resourceIDForApp(name string) string {
	return "service:" + name
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
}

type metricsResponse struct {
	Metric string        `json:"metric"`
	Points []metricPoint `json:"points"`
}

// handleQueryMetrics handles GET /api/v1/apps/{name}/metrics.
// Query params: metric (required), from/to (RFC3339, default now-1h to
// now), step (a Go duration string like "60s"; omitted or "0" means raw,
// unaggregated samples, per telemetry.Aggregate's own step<=0 contract).
func (rt *Router) handleQueryMetrics(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}

	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: query metrics: load app failed", slog.String("error", err.Error()), slog.String("name", name))
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

	samples, err := rt.telemetry.QueryMetrics(r.Context(), resourceIDForApp(name), metric, from, to)
	if err != nil {
		// A federated query with a partial result (ADR 008: some agents
		// unreachable) still has real data to return; only a total
		// failure (every source errored, samples empty and err set) is
		// treated as a request failure here. Federator.QueryMetrics
		// doesn't distinguish "partial" from "total" in its return
		// shape, so this checks the one signal available: whether
		// anything at all came back.
		if len(samples) == 0 {
			rt.logger.Error("api: query metrics failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("metric", metric))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		rt.logger.Warn("api: query metrics: partial result", slog.String("error", err.Error()), slog.String("name", name), slog.String("metric", metric))
	}

	aggregated := telemetry.Aggregate(samples, from, step)
	points := make([]metricPoint, len(aggregated))
	for i, a := range aggregated {
		points[i] = metricPoint{Timestamp: a.Timestamp, Value: a.Value, Count: a.Count}
	}

	writeJSON(w, http.StatusOK, metricsResponse{Metric: metric, Points: points})
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
