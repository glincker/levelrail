package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/deploytrace"
	"github.com/GLINCKER/levelrail/internal/store"
)

// deployStagesResponse is the wire shape of GET .../deploys/{deployId}/stages.
type deployStagesResponse struct {
	DeployID string                  `json:"deploy_id"`
	Status   string                  `json:"status"`
	Stages   []deploytrace.StageView `json:"stages"`
	// WallMS is the attempt's elapsed time; StagesMS sums the stage durations, so a viewer can check they add up.
	WallMS   int64 `json:"wall_ms"`
	StagesMS int64 `json:"stages_ms"`
}

// loadStageAttempt checks read access and that the attempt belongs to the app.
func (rt *Router) loadStageAttempt(w http.ResponseWriter, r *http.Request) (*store.DeployAttempt, bool) {
	name := r.PathValue("name")
	if !rt.requireReadableApp(w, r, name) {
		return nil, false
	}
	deployID := r.PathValue("deployId")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
		} else {
			rt.internalError(w, "api: deploy stages: load app failed", err, slog.String("name", name))
		}
		return nil, false
	}
	attempt, err := rt.deployAttempts.GetDeployAttempt(r.Context(), deployID)
	if errors.Is(err, store.ErrDeployAttemptNotFound) {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return nil, false
	}
	if err != nil {
		rt.internalError(w, "api: deploy stages: load attempt failed", err, slog.String("deploy_id", deployID))
		return nil, false
	}
	if attempt.ServiceName != name {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return nil, false
	}
	return attempt, true
}

// handleGetDeployStages handles GET /api/v1/apps/{name}/deploys/{deployId}/stages:
// the build stages with start, end, cache state and log size. Plain JSON, so
// the dashboard and the CLI poll it while a build runs.
func (rt *Router) handleGetDeployStages(w http.ResponseWriter, r *http.Request) {
	attempt, ok := rt.loadStageAttempt(w, r)
	if !ok {
		return
	}
	stages := deploytrace.StagesFromRows(rt.traceRows(r.Context(), attempt.ID))
	if stages == nil {
		stages = []deploytrace.StageView{}
	}
	end := time.Now()
	if attempt.FinishedAt != nil {
		end = *attempt.FinishedAt
	}
	resp := deployStagesResponse{DeployID: attempt.ID, Status: attempt.Status, Stages: stages, WallMS: end.Sub(attempt.StartedAt).Milliseconds()}
	for _, s := range stages {
		resp.StagesMS += s.DurationMS
	}
	writeJSON(w, http.StatusOK, resp)
}

// serveStageLog streams only one stage's lines: live from the recorder while
// the build runs, from the stored stage tail afterwards.
func (rt *Router) serveStageLog(w http.ResponseWriter, r *http.Request, attempt *store.DeployAttempt, stage string) {
	flusher, ok := rt.startLiveSSE(w, r)
	if !ok {
		rt.logger.Error("api: deploy stage log: response writer does not support flushing", slog.String("deploy_id", attempt.ID))
		return
	}
	_, _ = fmt.Fprint(w, ": connected\n\n")
	masked := newMaskedSSE(w)

	if attempt.FinishedAt == nil && rt.deployRecorder != nil {
		if lines, live, unsubscribe, ok := rt.deployRecorder.Snapshot(attempt.ID); ok {
			defer unsubscribe()
			for _, ev := range lines {
				if ev.Stage == stage {
					masked.Write(sseLogEvent{Line: ev.Line, Stream: ev.Stream, At: eventAt(ev.At)})
				}
			}
			flusher.Flush()
			rt.followStageLog(r.Context(), live, masked, flusher, stage)
			return
		}
	}
	rt.writeStoredStageTail(r.Context(), masked, attempt.ID, stage)
	masked.Flush()
	flusher.Flush()
	<-r.Context().Done()
}

func (rt *Router) followStageLog(ctx context.Context, live <-chan deploylog.Event, masked *maskedSSE, flusher http.Flusher, stage string) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-masked.Idle():
			masked.Flush()
			flusher.Flush()
		case ev, open := <-live:
			if !open {
				masked.Flush()
				flusher.Flush()
				<-ctx.Done()
				return
			}
			if ev.Stage != stage {
				continue
			}
			masked.Write(sseLogEvent{Line: ev.Line, Stream: ev.Stream, At: eventAt(ev.At)})
			flusher.Flush()
		}
	}
}

func (rt *Router) writeStoredStageTail(ctx context.Context, masked *maskedSSE, attemptID, stage string) {
	for _, line := range deploytrace.StageTail(rt.traceRows(ctx, attemptID), stage) {
		masked.Write(sseLogEvent{Line: line, Stream: "stdout"})
	}
}
