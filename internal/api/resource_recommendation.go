package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/diagnose"
	"github.com/GLINCKER/levelrail/internal/rightsizing"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// defaultResourceRecommendationLookback is how far back
// handleAppResourceRecommendation looks for usage history when
// WithResourceRecommendationLookback isn't configured.
const defaultResourceRecommendationLookback = 7 * 24 * time.Hour

type dimensionRecommendationResource struct {
	Dimension      string  `json:"dimension"`
	SampleCount    int     `json:"sample_count"`
	DataSufficient bool    `json:"data_sufficient"`
	Confidence     string  `json:"confidence"`
	CurrentLimit   int64   `json:"current_limit"`
	P95Usage       float64 `json:"p95_usage"`
	P99Usage       float64 `json:"p99_usage"`
	SuggestedLimit int64   `json:"suggested_limit"`
	Action         string  `json:"action,omitempty"`
	Reason         string  `json:"reason"`
}

type resourceRecommendationResource struct {
	ServiceName    string                          `json:"service_name"`
	LookbackWindow string                          `json:"lookback_window"`
	Memory         dimensionRecommendationResource `json:"memory"`
	CPU            dimensionRecommendationResource `json:"cpu"`
	OOMDetectedAt  string                          `json:"oom_detected_at,omitempty"`
	OOMExcerpt     string                          `json:"oom_excerpt,omitempty"`
}

func toDimensionRecommendationResource(d rightsizing.DimensionRecommendation) dimensionRecommendationResource {
	return dimensionRecommendationResource{
		Dimension:      d.Dimension,
		SampleCount:    d.SampleCount,
		DataSufficient: d.DataSufficient,
		Confidence:     d.Confidence,
		CurrentLimit:   d.CurrentLimit,
		P95Usage:       d.P95Usage,
		P99Usage:       d.P99Usage,
		SuggestedLimit: d.SuggestedLimit,
		Action:         d.Action,
		Reason:         d.Reason,
	}
}

func toResourceRecommendationResource(res rightsizing.Result) resourceRecommendationResource {
	out := resourceRecommendationResource{
		ServiceName:    res.ServiceName,
		LookbackWindow: res.LookbackWindow.String(),
		Memory:         toDimensionRecommendationResource(res.Memory),
		CPU:            toDimensionRecommendationResource(res.CPU),
	}
	if res.OOM != nil {
		out.OOMDetectedAt = res.OOM.DetectedAt.UTC().Format(time.RFC3339)
		out.OOMExcerpt = res.OOM.Excerpt
	}
	return out
}

// handleAppResourceRecommendation handles GET
// /api/v1/apps/{name}/resource-recommendation: a read-only, deterministic
// suggestion (internal/rightsizing) for this app's memory and CPU limits,
// derived from its own historical usage samples and, when found, a real
// OOM-kill log signal. Per CLAUDE.md section 4.11, this is a
// read-and-suggest layer only: it never writes to any resource, never
// calls an external model, and the suggestion is never applied
// automatically.
func (rt *Router) handleAppResourceRecommendation(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}

	name := r.PathValue("name")
	ctx := r.Context()

	svc, err := rt.apps.GetDesiredService(ctx, name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: resource recommendation: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	lookback := rt.resourceRecommendationLookback
	if lookback <= 0 {
		lookback = defaultResourceRecommendationLookback
	}

	now := time.Now()
	from := now.Add(-lookback)
	resourceID := resourceIDForApp(name)

	memSamples, err := rt.telemetry.QueryMetrics(ctx, resourceID, "memory_usage_bytes", from, now)
	if err != nil {
		rt.logger.Warn("api: resource recommendation: query memory metrics failed", slog.String("error", err.Error()), slog.String("name", name))
	}
	cpuSamples, err := rt.telemetry.QueryMetrics(ctx, resourceID, "cpu_percent", from, now)
	if err != nil {
		rt.logger.Warn("api: resource recommendation: query cpu metrics failed", slog.String("error", err.Error()), slog.String("name", name))
	}

	var currentMemory, currentNanoCPUs int64
	if svc.Resources != nil {
		currentMemory = svc.Resources.MemoryBytes
		currentNanoCPUs = svc.Resources.NanoCPUs
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

func toRightsizingSamples(samples []telemetry.Sample) []rightsizing.Sample {
	out := make([]rightsizing.Sample, len(samples))
	for i, s := range samples {
		out[i] = rightsizing.Sample{Timestamp: s.Timestamp, Value: s.Value}
	}
	return out
}

// findOOMEvidence looks for the most recent log line matching a known
// OOM-kill pattern (diagnose.OOMLogPatterns, the same signal
// handleDiagnoseApp's own possible_oom_kill signature already relies on)
// within [from, to]. Best-effort: no telemetry, or a query failure for
// one pattern, just means that pattern contributes no evidence, never a
// reason to fail the whole recommendation.
func (rt *Router) findOOMEvidence(ctx context.Context, name, resourceID string, from, to time.Time) *rightsizing.OOMEvidence {
	var best *telemetry.LogEntry
	for _, pattern := range diagnose.OOMLogPatterns {
		entries, err := rt.telemetry.QueryLogs(ctx, resourceID, from, to, pattern)
		if err != nil {
			rt.logger.Warn("api: resource recommendation: query oom logs failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("pattern", pattern))
			continue
		}
		for i := range entries {
			e := &entries[i]
			if best == nil || e.Timestamp.After(best.Timestamp) {
				best = e
			}
		}
	}
	if best == nil {
		return nil
	}
	return &rightsizing.OOMEvidence{DetectedAt: best.Timestamp, Excerpt: best.Message}
}
