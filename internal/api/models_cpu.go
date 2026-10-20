package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/models"
)

// ModelCPUService is the CPU-mode surface of the model service, asserted
// at request time so a service without it answers 501.
type ModelCPUService interface {
	LocalCatalogView(ctx context.Context, nodeID string) (models.CatalogView, error)
	CPUFitCheck(ctx context.Context, req models.CPUFitRequest) (models.CPUFitReport, error)
	Benchmark(ctx context.Context, name string) (models.BenchResult, error)
}

func (rt *Router) cpuModels(w http.ResponseWriter) (ModelCPUService, bool) {
	if !rt.modelsConfigured(w) {
		return nil, false
	}
	svc, ok := rt.models.(ModelCPUService)
	if !ok {
		writeError(w, http.StatusNotImplemented, "CPU models are not available on this control plane")
		return nil, false
	}
	return svc, true
}

type modelDownloadResource struct {
	DoneBytes  int64 `json:"done_bytes"`
	TotalBytes int64 `json:"total_bytes"`
	Percent    int   `json:"percent"`
}

type modelCPUResource struct {
	Compute        string                 `json:"compute"`
	CPULimit       float64                `json:"cpu_limit,omitempty"`
	MemoryLimitMiB int                    `json:"memory_limit_mib,omitempty"`
	WeightsRepo    string                 `json:"weights_repo,omitempty"`
	WeightsFile    string                 `json:"weights_file,omitempty"`
	LicenseAck     bool                   `json:"license_acknowledged,omitempty"`
	Download       *modelDownloadResource `json:"download,omitempty"`
	Benchmark      *models.BenchResult    `json:"benchmark,omitempty"`
}

func toModelCPUResource(v models.View) modelCPUResource {
	m := v.Model
	out := modelCPUResource{Compute: m.Compute, Benchmark: models.ParseBenchmark(m.BenchmarkJSON)}
	if out.Compute == "" {
		out.Compute = models.ComputeGPU
	}
	if m.Compute != models.ComputeCPU {
		return out
	}
	out.CPULimit, out.MemoryLimitMiB = float64(m.CPUMilli)/1000, m.MemoryMiB
	out.WeightsRepo, out.WeightsFile, out.LicenseAck = m.WeightsRepo, m.WeightsFile, m.LicenseAck
	if m.DownloadTotalBytes > 0 && !v.Ready {
		out.Download = &modelDownloadResource{DoneBytes: m.DownloadDoneBytes, TotalBytes: m.DownloadTotalBytes,
			Percent: int(m.DownloadDoneBytes * 100 / m.DownloadTotalBytes)}
	}
	return out
}

// handleModelCatalog handles GET /api/v1/model-catalog.
func (rt *Router) handleModelCatalog(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.cpuModels(w)
	if !ok {
		return
	}
	nodeID := r.URL.Query().Get("node_id")
	if nodeID != "" && !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusBadRequest, "unknown node_id"), nodeRef(nodeID)) {
		return
	}
	view, err := svc.LocalCatalogView(r.Context(), nodeID)
	if err != nil {
		rt.writeModelError(w, "read local model catalog", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// handleModelBenchmark handles POST /api/v1/models/{name}/benchmark.
func (rt *Router) handleModelBenchmark(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.cpuModels(w)
	if !ok {
		return
	}
	res, err := svc.Benchmark(r.Context(), r.PathValue("name"))
	var limited *models.BenchRateLimitError
	switch {
	case errors.As(err, &limited):
		w.Header().Set("Retry-After", strconv.Itoa(int(limited.RetryAfter.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, models.ErrBenchDisabled):
		writeError(w, http.StatusNotImplemented, err.Error())
	case errors.Is(err, models.ErrBenchBusy):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, models.ErrBenchNotReady), errors.Is(err, models.ErrBenchEmbedding):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		rt.writeModelError(w, "benchmark model", err)
	default:
		writeJSON(w, http.StatusOK, res)
	}
}
