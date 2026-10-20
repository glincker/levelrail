package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/rightsizing"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Database counterpart of handleAppResourceRecommendation; only the store lookup and resource ID differ,
// since rightsizing is resource-agnostic.
func (rt *Router) handleDatabaseResourceRecommendation(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}

	name := r.PathValue("name")
	ctx := r.Context()

	dbSpec, err := rt.databases.GetDesiredDatabase(ctx, name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: database resource recommendation: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	lookback := rt.resourceRecommendationLookback
	if lookback <= 0 {
		lookback = defaultResourceRecommendationLookback
	}

	now := time.Now()
	from := now.Add(-lookback)
	resourceID := resourceIDForDatabase(name)

	memSamples, err := rt.telemetry.QueryMetrics(ctx, resourceID, "memory_usage_bytes", from, now)
	if err != nil {
		rt.logger.Warn("api: database resource recommendation: query memory metrics failed", slog.String("error", err.Error()), slog.String("name", name))
	}
	cpuSamples, err := rt.telemetry.QueryMetrics(ctx, resourceID, "cpu_percent", from, now)
	if err != nil {
		rt.logger.Warn("api: database resource recommendation: query cpu metrics failed", slog.String("error", err.Error()), slog.String("name", name))
	}

	var currentMemory, currentNanoCPUs int64
	if dbSpec.Resources != nil {
		currentMemory = dbSpec.Resources.MemoryBytes
		currentNanoCPUs = dbSpec.Resources.NanoCPUs
	}

	result := rightsizing.Recommend(rightsizing.Input{
		ServiceName:        name,
		Now:                now,
		LookbackWindow:     lookback,
		MemorySamples:      toRightsizingSamples(memSamples),
		CPUPercentSamples:  toRightsizingSamples(cpuSamples),
		CurrentMemoryBytes: currentMemory,
		CurrentNanoCPUs:    currentNanoCPUs,
		OOM:                rt.findOOMEvidence(ctx, name, resourceID, from, now),
	})

	writeJSON(w, http.StatusOK, toResourceRecommendationResource(result))
}
