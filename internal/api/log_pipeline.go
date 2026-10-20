package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	maxLogPipelineBody    = 256 << 10
	maxLogPipelinePreview = 200
)

// LogPipelineStore is the per-app log pipeline storage the routes need.
type LogPipelineStore interface {
	GetLogPipeline(ctx context.Context, resourceID string) ([]telemetry.PipelineRule, error)
	SetLogPipeline(ctx context.Context, resourceID string, rules []telemetry.PipelineRule) error
	DeleteLogPipeline(ctx context.Context, resourceID string) error
}

// WithLogPipelineStore turns on the app log pipeline routes.
func WithLogPipelineStore(s LogPipelineStore) Option {
	return func(rt *Router) { rt.logPipelines = s }
}

type logPipelineResource struct {
	Rules []telemetry.PipelineRule `json:"rules"`
}

type logPipelinePreviewRequest struct {
	Rules []telemetry.PipelineRule `json:"rules"`
	Lines []string                 `json:"lines"`
}

type logPipelinePreviewResponse struct {
	Lines []telemetry.PreviewLine `json:"lines"`
}

func (rt *Router) logPipelineReady(w http.ResponseWriter) bool {
	if rt.logPipelines == nil {
		writeError(w, http.StatusNotImplemented, "log pipelines are not configured on this control plane")
		return false
	}
	return true
}

func decodeLogPipelineBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLogPipelineBody)).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}

// handleGetLogPipeline handles GET /api/v1/apps/{name}/log-pipeline.
func (rt *Router) handleGetLogPipeline(w http.ResponseWriter, r *http.Request) {
	if !rt.logPipelineReady(w) {
		return
	}
	name := r.PathValue("name")
	rules, err := rt.logPipelines.GetLogPipeline(r.Context(), "service:"+name)
	if err != nil {
		rt.internalError(w, "api: get log pipeline", err, slog.String("name", name))
		return
	}
	if rules == nil {
		rules = []telemetry.PipelineRule{}
	}
	writeJSON(w, http.StatusOK, logPipelineResource{Rules: rules})
}

// handleSetLogPipeline handles PUT /api/v1/apps/{name}/log-pipeline.
func (rt *Router) handleSetLogPipeline(w http.ResponseWriter, r *http.Request) {
	if !rt.logPipelineReady(w) {
		return
	}
	var req logPipelineResource
	if !decodeLogPipelineBody(w, r, &req) {
		return
	}
	if err := telemetry.ValidatePipeline(req.Rules); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := r.PathValue("name")
	if err := rt.logPipelines.SetLogPipeline(r.Context(), "service:"+name, req.Rules); err != nil {
		rt.internalError(w, "api: set log pipeline", err, slog.String("name", name))
		return
	}
	if req.Rules == nil {
		req.Rules = []telemetry.PipelineRule{}
	}
	writeJSON(w, http.StatusOK, req)
}

// handleDeleteLogPipeline handles DELETE /api/v1/apps/{name}/log-pipeline.
func (rt *Router) handleDeleteLogPipeline(w http.ResponseWriter, r *http.Request) {
	if !rt.logPipelineReady(w) {
		return
	}
	name := r.PathValue("name")
	if err := rt.logPipelines.DeleteLogPipeline(r.Context(), "service:"+name); err != nil {
		rt.internalError(w, "api: delete log pipeline", err, slog.String("name", name))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePreviewLogPipeline handles POST /api/v1/apps/{name}/log-pipeline/preview.
func (rt *Router) handlePreviewLogPipeline(w http.ResponseWriter, r *http.Request) {
	if !rt.logPipelineReady(w) {
		return
	}
	var req logPipelinePreviewRequest
	if !decodeLogPipelineBody(w, r, &req) {
		return
	}
	if len(req.Lines) > maxLogPipelinePreview {
		writeError(w, http.StatusBadRequest, "preview takes at most 200 lines")
		return
	}
	out, err := telemetry.PreviewPipeline(req.Rules, req.Lines)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if out == nil {
		out = []telemetry.PreviewLine{}
	}
	writeJSON(w, http.StatusOK, logPipelinePreviewResponse{Lines: out})
}
