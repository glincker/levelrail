package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/models"
)

// ModelPreflightService is the optional surface behind the Hugging Face
// preflight and model cache routes. The models service implements it.
type ModelPreflightService interface {
	PreflightEnabled() bool
	Preflight(ctx context.Context, in models.PreflightInput) (models.PreflightResult, error)
	CacheEnabled() bool
	CacheList(ctx context.Context) (models.CacheReport, error)
	CachePrune(ctx context.Context, volumes []string, dryRun bool) (models.CachePruneResult, error)
}

type modelPreflightRequest struct {
	Repo   string `json:"repo"`
	Engine string `json:"engine"`
	File   string `json:"file"`
	Quant  string `json:"quant"`
	NodeID string `json:"node_id"`
	// HFToken is used for this request only, never stored or logged.
	HFToken string `json:"hf_token"`
}

type modelCachePruneRequest struct {
	Volumes []string `json:"volumes"`
	DryRun  bool     `json:"dry_run"`
}

func (rt *Router) preflightService(w http.ResponseWriter) (ModelPreflightService, bool) {
	if !rt.modelsConfigured(w) {
		return nil, false
	}
	svc, ok := rt.models.(ModelPreflightService)
	if !ok {
		writeError(w, http.StatusNotImplemented, "model preflight is not available on this control plane")
		return nil, false
	}
	return svc, true
}

// handleModelPreflight handles POST /api/v1/models/preflight. Hub
// problems (missing, gated, rate limited) are a 200 with a status, so a
// caller can always render the next step.
func (rt *Router) handleModelPreflight(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.preflightService(w)
	if !ok {
		return
	}
	if !svc.PreflightEnabled() {
		writeError(w, http.StatusNotImplemented, "Hugging Face preflight is not configured")
		return
	}
	var req modelPreflightRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := svc.Preflight(r.Context(), models.PreflightInput{
		Repo: req.Repo, Engine: req.Engine, File: strings.TrimSpace(req.File), Quant: strings.TrimSpace(req.Quant),
		NodeID: req.NodeID, Token: strings.TrimSpace(req.HFToken),
	})
	if err != nil {
		rt.writeModelError(w, "model preflight", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleListModelCache handles GET /api/v1/model-cache.
func (rt *Router) handleListModelCache(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.preflightService(w)
	if !ok {
		return
	}
	if !svc.CacheEnabled() {
		writeError(w, http.StatusNotImplemented, "the model cache manager is not configured")
		return
	}
	report, err := svc.CacheList(r.Context())
	if err != nil {
		rt.writeModelError(w, "list model cache", err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// handlePruneModelCache handles POST /api/v1/model-cache/prune. It never
// removes a volume that is mounted or belongs to a configured model.
func (rt *Router) handlePruneModelCache(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.preflightService(w)
	if !ok {
		return
	}
	if !svc.CacheEnabled() {
		writeError(w, http.StatusNotImplemented, "the model cache manager is not configured")
		return
	}
	var req modelCachePruneRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := svc.CachePrune(r.Context(), req.Volumes, req.DryRun)
	if err != nil {
		rt.writeModelError(w, "prune model cache", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
