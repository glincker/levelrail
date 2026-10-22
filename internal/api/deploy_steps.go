package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/store"
)

// This file adds a step-level view of a build/deploy attempt alongside
// deploy_attempts.go's own raw log stream: discrete, named pipeline
// phases (detecting, building, pushing, deploying) with timestamps,
// carried over deploylog.Recorder's own step fan-out (see that type's
// Step/SnapshotSteps doc comments), so the frontend can render a step
// list with a spinner/checkmark per phase instead of only a scrolling
// log. It is purely an additional observability view over the same
// synchronous build-trigger flow builds.go already runs: it introduces
// no new signal into the reconciler, which stays level-triggered and
// unaware this stream exists, per 4.2.

// sseStepEvent is the wire shape for one step transition.
type sseStepEvent struct {
	Step      string `json:"step"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	// The fields below are set for traced steps only (deploytrace), so a
	// build step keeps its original three-field shape.
	Key        string `json:"key,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Detail     string `json:"detail,omitempty"`
	Service    string `json:"service,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	EndedAt    string `json:"ended_at,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	Attempts   int    `json:"attempts,omitempty"`
}

func toSSEStepEvent(ev deploylog.StepEvent) sseStepEvent {
	return sseStepEvent{Step: ev.Step, Status: ev.Status, Timestamp: ev.Timestamp.UTC().Format(time.RFC3339Nano)}
}

// handleDeployStepStream handles
// GET /api/v1/apps/{name}/deploys/{deployId}/steps: an SSE stream of one
// deploy attempt's named pipeline steps, live from rt.deployRecorder if
// still running, or a small synthesized replay if already finished (see
// serveFinishedDeploySteps's own doc comment for why that replay is not
// a full step-by-step history). 501 if rt.deployRecorder isn't
// configured, matching handleDeployLogStream's own shape for the
// identical reason.
func (rt *Router) handleDeployStepStream(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	deployID := r.PathValue("deployId")

	_, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: deploy step stream: load app failed", err, slog.String("name", name))
		return
	}

	attempt, err := rt.deployAttempts.GetDeployAttempt(r.Context(), deployID)
	if errors.Is(err, store.ErrDeployAttemptNotFound) {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: deploy step stream: load attempt failed", err, slog.String("deploy_id", deployID))
		return
	}
	if attempt.ServiceName != name {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return
	}

	if rt.deployRecorder == nil {
		writeError(w, http.StatusNotImplemented, "live deploy step streaming is not configured on this control plane")
		return
	}

	if attempt.FinishedAt != nil {
		rt.serveFinishedDeploySteps(r.Context(), w, *attempt)
		return
	}
	rt.serveLiveDeploySteps(w, r, deployID)
}

// serveFinishedDeploySteps writes a two-point synthesis of attempt's
// outcome (started, then its terminal status) as one SSE burst, then
// holds the connection open until ctx is done, the same reconnect-
// avoidance shape serveFinishedDeployLog uses. Not a full step replay:
// deploylog.Recorder never persists step events (see Step's own doc
// comment), only the attempt row's own started_at/finished_at/status
// survive past Finish, so a caller reconnecting after the attempt ended
// sees only these two synthesized points, not each intermediate phase.
func (rt *Router) serveFinishedDeploySteps(ctx context.Context, w http.ResponseWriter, attempt store.DeployAttempt) {
	if rows := rt.traceRows(ctx, attempt.ID); len(rows) > 0 {
		rt.serveTraceSteps(ctx, w, attempt.ID, rows, true)
		return
	}
	flusher, ok := startSSE(w)
	if !ok {
		rt.logger.Error("api: deploy step stream: response writer does not support flushing", slog.String("deploy_id", attempt.ID))
		return
	}
	_, _ = fmt.Fprint(w, ": connected\n\n")

	writeSSEStepEvent(w, sseStepEvent{Step: "building", Status: "running", Timestamp: attempt.StartedAt.UTC().Format(time.RFC3339Nano)})
	finalStep, finalStatus := "deploying", "done"
	finishedAt := attempt.StartedAt
	if attempt.FinishedAt != nil {
		finishedAt = *attempt.FinishedAt
	}
	if attempt.Status == store.DeployAttemptStatusFailed {
		finalStep, finalStatus = "failed", "failed"
	}
	writeSSEStepEvent(w, sseStepEvent{Step: finalStep, Status: finalStatus, Timestamp: finishedAt.UTC().Format(time.RFC3339Nano)})
	flusher.Flush()

	<-ctx.Done()
}

// serveLiveDeploySteps serves deployID's in-progress steps: whatever
// already happened, then a live tail until Finish closes the channel or
// the client disconnects.
func (rt *Router) serveLiveDeploySteps(w http.ResponseWriter, r *http.Request, deployID string) {
	steps, live, unsubscribe, ok := rt.deployRecorder.SnapshotSteps(deployID)
	if !ok {
		// The recorder has no record of this attempt (e.g. control plane
		// restarted mid-build, see FailOrphanedDeployAttempts): fall back
		// to the attempt row's own already-persisted terminal state.
		attempt, err := rt.deployAttempts.GetDeployAttempt(r.Context(), deployID)
		if err != nil {
			rt.internalError(w, "api: deploy step stream: load attempt failed", err, slog.String("deploy_id", deployID))
			return
		}
		rt.serveFinishedDeploySteps(r.Context(), w, *attempt)
		return
	}
	defer unsubscribe()

	flusher, ok := rt.startLiveSSE(w, r)
	if !ok {
		rt.logger.Error("api: deploy step stream: response writer does not support flushing", slog.String("deploy_id", deployID))
		return
	}
	_, _ = fmt.Fprint(w, ": connected\n\n")
	for _, ev := range steps {
		writeSSEStepEvent(w, toSSEStepEvent(ev))
	}
	sent := map[string]string{}
	rt.writeTraceExtras(r.Context(), w, deployID, sent)
	flusher.Flush()

	tick := time.NewTicker(durationEnv(EnvTracePollInterval, time.Second))
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			if rt.writeTraceExtras(r.Context(), w, deployID, sent) {
				flusher.Flush()
			}
		case ev, chOpen := <-live:
			if !chOpen {
				// The build recorder is done; the reconciler keeps adding
				// pull, probe and cutover steps to the stored trace.
				rt.writeTraceExtras(r.Context(), w, deployID, sent)
				flusher.Flush()
				rt.followTrace(r.Context(), deployID, true, sent, func(changed []store.DeployTraceStep) {
					for _, row := range changed {
						writeSSEStepEvent(w, toSSETraceEvent(row))
					}
					flusher.Flush()
				})
				<-r.Context().Done()
				return
			}
			writeSSEStepEvent(w, toSSEStepEvent(ev))
			flusher.Flush()
		}
	}
}

func writeSSEStepEvent(w http.ResponseWriter, ev sseStepEvent) {
	data, err := json.Marshal(ev)
	if err != nil {
		slog.Default().Error("api: marshal deploy step event failed", slog.String("error", err.Error()))
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}
