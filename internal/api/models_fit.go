package api

import (
	"encoding/json"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/models"
)

type modelFitRequest struct {
	Engine        string   `json:"engine"`
	Model         string   `json:"model"`
	Quantization  string   `json:"quantization"`
	ContextLength int      `json:"context_length"`
	GPUCount      int      `json:"gpu_count"`
	GPUDeviceIDs  []string `json:"gpu_device_ids"`
	WeightsBytes  int64    `json:"weights_bytes"`
	// NodeID limits the check to one node ("" is the control plane's own
	// host); omit it to rate every GPU node.
	NodeID *string `json:"node_id"`
	// Compute "cpu" (or a catalog_id) rates the model against the node's
	// CPUs and memory instead of GPU VRAM.
	Compute   string  `json:"compute"`
	CatalogID string  `json:"catalog_id"`
	CPUs      float64 `json:"cpus"`
	MemoryMiB int     `json:"memory_mib"`
}

// handleModelFitCheck handles POST /api/v1/models/fit. It only reads, so a
// read ability suffices even though it is a POST.
func (rt *Router) handleModelFitCheck(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	var req modelFitRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.WeightsBytes < 0 || req.ContextLength < 0 {
		writeError(w, http.StatusBadRequest, "weights_bytes and context_length must not be negative")
		return
	}
	if req.NodeID != nil && !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusBadRequest, "unknown node_id"), nodeRef(*req.NodeID)) {
		return
	}
	if req.Compute == models.ComputeCPU || req.CatalogID != "" {
		rt.handleCPUFitCheck(w, r, req)
		return
	}
	rep, err := rt.models.FitCheck(r.Context(), models.FitRequest{
		Engine: req.Engine, ModelRef: req.Model, Quantization: req.Quantization, ContextLength: req.ContextLength,
		GPUCount: req.GPUCount, GPUDeviceIDs: req.GPUDeviceIDs, WeightsBytes: req.WeightsBytes, NodeID: req.NodeID,
	})
	if err != nil {
		rt.writeModelError(w, "check model fit", err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// handleModelFit handles GET /api/v1/models/{name}/fit.
func (rt *Router) handleModelFit(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	rep, err := rt.models.ModelFit(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.writeModelError(w, "check model fit", err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (rt *Router) handleCPUFitCheck(w http.ResponseWriter, r *http.Request, req modelFitRequest) {
	svc, ok := rt.cpuModels(w)
	if !ok {
		return
	}
	if req.CPUs < 0 || req.MemoryMiB < 0 {
		writeError(w, http.StatusBadRequest, "cpus and memory_mib must not be negative")
		return
	}
	nodeID := ""
	if req.NodeID != nil {
		nodeID = *req.NodeID
	}
	rep, err := svc.CPUFitCheck(r.Context(), models.CPUFitRequest{CatalogID: req.CatalogID, ModelRef: req.Model, Quantization: req.Quantization,
		ContextLength: req.ContextLength, WeightsBytes: req.WeightsBytes, NodeID: nodeID, CPUMilli: int(req.CPUs * 1000), MemoryMiB: req.MemoryMiB})
	if err != nil {
		rt.writeModelError(w, "check cpu model fit", err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
