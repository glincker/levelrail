package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/costestimate"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// costEstimateObservedLookback is how far back handleAppCostEstimate
// looks for usage history when an app has no declared resource limit to
// estimate from, the same 7-day window defaultResourceRecommendationLookback
// uses for the same reason: long enough to see a real p95, short enough
// to still reflect current behavior.
const costEstimateObservedLookback = 7 * 24 * time.Hour

type costEstimateProviderResource struct {
	Key            string  `json:"key"`
	Label          string  `json:"label"`
	CPUCostUSD     float64 `json:"cpu_cost_usd"`
	MemoryCostUSD  float64 `json:"memory_cost_usd"`
	TotalUSD       float64 `json:"total_usd"`
	MinimumApplied bool    `json:"minimum_applied"`
}

// costEstimateResource is GET /api/v1/apps/{name}/cost-estimate's wire
// shape. This is an ESTIMATE, not a real bill: Note always restates
// that so no consumer of this endpoint mistakes it for an invoice.
type costEstimateResource struct {
	ServiceName string                         `json:"service_name"`
	VCPUCores   float64                        `json:"vcpu_cores"`
	MemoryGiB   float64                        `json:"memory_gib"`
	CPUBasis    string                         `json:"cpu_basis"`
	MemoryBasis string                         `json:"memory_basis"`
	Providers   []costEstimateProviderResource `json:"providers"`
	Note        string                         `json:"note"`
}

const costEstimateNote = "Estimate only, not a real bill: it compares this app's declared or observed CPU/memory against illustrative reference pricing, not any live provider quote. Correct the rates for your own region with APP_COST_ESTIMATE_* env vars; see docs/cost-estimate.md."

func toCostEstimateResource(res costestimate.Result) costEstimateResource {
	providers := make([]costEstimateProviderResource, 0, len(res.Providers))
	for _, p := range res.Providers {
		providers = append(providers, costEstimateProviderResource{
			Key:            p.Key,
			Label:          p.Label,
			CPUCostUSD:     p.CPUCostUSD,
			MemoryCostUSD:  p.MemoryCostUSD,
			TotalUSD:       p.TotalUSD,
			MinimumApplied: p.MinimumApplied,
		})
	}
	return costEstimateResource{
		ServiceName: res.ServiceName,
		VCPUCores:   res.VCPUCores,
		MemoryGiB:   res.MemoryGiB,
		CPUBasis:    res.CPUBasis,
		MemoryBasis: res.MemoryBasis,
		Providers:   providers,
		Note:        costEstimateNote,
	}
}

// handleAppCostEstimate handles GET /api/v1/apps/{name}/cost-estimate: a
// read-only, deterministic "what would this app cost elsewhere" estimate
// (internal/costestimate) derived from the app's declared CPU/memory
// limits, falling back to observed usage history only when no limit is
// configured. Unlike resource-recommendation, this never requires
// telemetry: an app with explicit resources: in app.yaml gets an
// estimate even on a control plane with telemetry turned off.
func (rt *Router) handleAppCostEstimate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()

	svc, err := rt.apps.GetDesiredService(ctx, name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: cost estimate: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var declaredMemory, declaredNanoCPUs int64
	if svc.Resources != nil {
		declaredMemory = svc.Resources.MemoryBytes
		declaredNanoCPUs = svc.Resources.NanoCPUs
	}

	var cpuSamples, memSamples []costestimate.Sample
	if rt.telemetry != nil && (declaredNanoCPUs <= 0 || declaredMemory <= 0) {
		now := time.Now()
		from := now.Add(-costEstimateObservedLookback)
		resourceID := resourceIDForApp(name)

		if declaredNanoCPUs <= 0 {
			samples, err := rt.telemetry.QueryMetrics(ctx, resourceID, "cpu_percent", from, now)
			if err != nil {
				rt.logger.Warn("api: cost estimate: query cpu metrics failed", slog.String("error", err.Error()), slog.String("name", name))
			}
			cpuSamples = toCostEstimateSamples(samples)
		}
		if declaredMemory <= 0 {
			samples, err := rt.telemetry.QueryMetrics(ctx, resourceID, "memory_usage_bytes", from, now)
			if err != nil {
				rt.logger.Warn("api: cost estimate: query memory metrics failed", slog.String("error", err.Error()), slog.String("name", name))
			}
			memSamples = toCostEstimateSamples(samples)
		}
	}

	result := costestimate.Estimate(costestimate.Input{
		ServiceName:                name,
		DeclaredNanoCPUs:           declaredNanoCPUs,
		DeclaredMemoryBytes:        declaredMemory,
		ObservedCPUPercentSamples:  cpuSamples,
		ObservedMemoryBytesSamples: memSamples,
	}, costestimate.RateTableFromEnv())

	writeJSON(w, http.StatusOK, toCostEstimateResource(result))
}

func toCostEstimateSamples(samples []telemetry.Sample) []costestimate.Sample {
	out := make([]costestimate.Sample, len(samples))
	for i, s := range samples {
		out[i] = costestimate.Sample{Timestamp: s.Timestamp, Value: s.Value}
	}
	return out
}
