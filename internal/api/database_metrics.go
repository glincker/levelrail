package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// handleQueryDatabaseMetrics handles GET /api/v1/databases/{name}/metrics:
// the database-kind counterpart to handleQueryMetrics (metrics.go), same
// query params (metric/from/to/step) and response shape, differing only
// in which store lookup confirms the resource exists and which
// resourceID key is queried.
func (rt *Router) handleQueryDatabaseMetrics(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}

	name := r.PathValue("name")

	if _, err := rt.databases.GetDesiredDatabase(r.Context(), name); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: query database metrics: load database failed", slog.String("error", err.Error()), slog.String("name", name))
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

	samples, err := rt.telemetry.QueryMetrics(r.Context(), resourceIDForDatabase(name), metric, from, to)
	if err != nil {
		if len(samples) == 0 {
			rt.logger.Error("api: query database metrics failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("metric", metric))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		rt.logger.Warn("api: query database metrics: partial result", slog.String("error", err.Error()), slog.String("name", name), slog.String("metric", metric))
	}

	aggregated := telemetry.Aggregate(samples, from, step)
	points := make([]metricPoint, len(aggregated))
	for i, a := range aggregated {
		points[i] = metricPoint{Timestamp: a.Timestamp, Value: a.Value, Count: a.Count}
	}

	writeJSON(w, http.StatusOK, metricsResponse{Metric: metric, Points: points})
}
