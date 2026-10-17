package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/store"
)

// PipelineExtraStore is the store surface for artifacts and inbound webhook
// triggers. *store.DB satisfies it; WithPipelines picks it up from the main
// pipeline store when that store implements it.
type PipelineExtraStore interface {
	GetPipeline(ctx context.Context, id string) (store.Pipeline, error)
	ListPipelineArtifacts(ctx context.Context, runID string) ([]store.PipelineArtifact, error)
	GetPipelineArtifact(ctx context.Context, id string) (store.PipelineArtifact, error)
	GetPipelineWebhookTrigger(ctx context.Context, pipelineID string) (store.PipelineWebhookTrigger, error)
	SavePipelineWebhookTrigger(ctx context.Context, t store.PipelineWebhookTrigger) error
	DeletePipelineWebhookTrigger(ctx context.Context, pipelineID string) error
	TouchPipelineWebhookTrigger(ctx context.Context, pipelineID string, at time.Time) error
	AddPipelineWebhookDelivery(ctx context.Context, d store.PipelineWebhookDelivery) (store.PipelineWebhookDelivery, error)
	FinishPipelineWebhookDelivery(ctx context.Context, id, outcome, reason, runID string, status int) error
	GetPipelineWebhookDelivery(ctx context.Context, pipelineID, id string) (store.PipelineWebhookDelivery, error)
	ListPipelineWebhookDeliveries(ctx context.Context, pipelineID string, limit int) ([]store.PipelineWebhookDelivery, error)
	PrunePipelineWebhookDeliveries(ctx context.Context, pipelineID string, keep int) error
}

// PipelineArtifactFiles opens a stored artifact after verifying its sha256.
// *pipeline.ArtifactStore satisfies it.
type PipelineArtifactFiles interface {
	Open(meta store.PipelineArtifact) (*os.File, error)
}

// SetPipelineArtifactFiles enables artifact downloads.
func (rt *Router) SetPipelineArtifactFiles(f PipelineArtifactFiles) { rt.pipelineArtifactFiles = f }

// SetPipelineWebhookLimits sets the inbound webhook body cap and how many
// deliveries each pipeline keeps. Zero values keep the defaults.
func (rt *Router) SetPipelineWebhookLimits(maxBody int64, keep int) {
	if maxBody > 0 {
		rt.pipelineWebhookMaxBody = maxBody
	}
	if keep > 0 {
		rt.pipelineWebhookKeep = keep
	}
}

type pipelineArtifactResource struct {
	ID        string    `json:"id"`
	Job       string    `json:"job"`
	Name      string    `json:"name"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func toArtifactResource(a store.PipelineArtifact) pipelineArtifactResource {
	return pipelineArtifactResource{ID: a.ID, Job: a.JobKey, Name: a.Name, SizeBytes: a.SizeBytes, SHA256: a.SHA256, CreatedAt: a.CreatedAt, ExpiresAt: a.ExpiresAt}
}

func (rt *Router) extrasReady(w http.ResponseWriter) bool {
	if !rt.pipelinesReady(w) {
		return false
	}
	if rt.pipelineExtras == nil {
		writeError(w, http.StatusNotImplemented, "pipeline artifacts and webhook triggers are not available on this control plane")
		return false
	}
	return true
}

// handleListPipelineRunArtifacts handles GET .../pipeline-runs/{id}/artifacts.
func (rt *Router) handleListPipelineRunArtifacts(w http.ResponseWriter, r *http.Request) {
	if !rt.extrasReady(w) {
		return
	}
	run, ok := rt.loadOwnedPipelineRun(w, r)
	if !ok {
		return
	}
	rows, err := rt.pipelineExtras.ListPipelineArtifacts(r.Context(), run.ID)
	if err != nil {
		rt.internalError(w, "api: list pipeline artifacts failed", err)
		return
	}
	out := make([]pipelineArtifactResource, 0, len(rows))
	for _, a := range rows {
		out = append(out, toArtifactResource(a))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDownloadPipelineArtifact handles GET .../artifacts/{aid}/download.
// The archive is hash-checked before the first byte is sent.
func (rt *Router) handleDownloadPipelineArtifact(w http.ResponseWriter, r *http.Request) {
	if !rt.extrasReady(w) {
		return
	}
	if rt.pipelineArtifactFiles == nil {
		writeError(w, http.StatusNotImplemented, "artifact storage is not configured on this control plane")
		return
	}
	run, ok := rt.loadOwnedPipelineRun(w, r)
	if !ok {
		return
	}
	meta, err := rt.pipelineExtras.GetPipelineArtifact(r.Context(), r.PathValue("aid"))
	if errors.Is(err, store.ErrPipelineArtifactNotFound) || (err == nil && meta.RunID != run.ID) {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: load pipeline artifact failed", err)
		return
	}
	f, err := rt.pipelineArtifactFiles.Open(meta)
	switch {
	case errors.Is(err, pipeline.ErrArtifactCorrupt):
		rt.logger.Error("api: pipeline artifact failed its sha256 check", "artifact_id", meta.ID, "run_id", run.ID)
		writeError(w, http.StatusConflict, "artifact failed its sha256 check and was not served")
		return
	case errors.Is(err, os.ErrNotExist):
		writeError(w, http.StatusGone, "artifact is no longer stored")
		return
	case err != nil:
		rt.internalError(w, "api: open pipeline artifact failed", err)
		return
	}
	defer func() { _ = f.Close() }()
	h := w.Header()
	h.Set("Content-Type", "application/gzip")
	h.Set("Content-Length", strconv.FormatInt(meta.SizeBytes, 10))
	h.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", artifactFilename(run.Number, meta.JobKey)))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Content-SHA256", meta.SHA256)
	h.Set("Cache-Control", "private, no-store")
	_, _ = io.Copy(w, f)
}

func artifactFilename(runNumber int, jobKey string) string {
	safe := make([]byte, 0, len(jobKey))
	for i := 0; i < len(jobKey); i++ {
		c := jobKey[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' {
			safe = append(safe, c)
		} else {
			safe = append(safe, '_')
		}
	}
	return fmt.Sprintf("run-%d-%s.tar.gz", runNumber, safe)
}
