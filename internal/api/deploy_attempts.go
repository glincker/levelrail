package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// This file is the real deploy-attempt history and log surface,
// implementing docs-local/research/deploy-attempt-id-and-log-persistence.md's
// recommendation: a row per real trigger call (deploys.go's
// handleTriggerDeploy, builds.go's handleTriggerBuild, and
// internal/webhook's own attempt tracking) in store.DeployAttempt, plus
// a full build/log stream over SSE that serves either a live tail (the
// attempt is still running) or a persisted replay (it already
// finished).
//
// This is deliberately additive to deploys.go's existing
// handleDeployHistory, not a replacement for it: that handler's response
// (the latest reconcile condition per controller/type) has a real
// existing frontend consumer (web/src/queries/deploys.ts's
// useDeployStatus, rendered by ConditionsPanel and read by
// AppScopedSidebar for the sidebar's status badge). Changing that
// endpoint's shape in place would silently break all three. A real
// attempt-history list belongs at its own URL instead.

// beginBuildDeployAttempt mints a deploy_attempts row for req (shared by
// handleTriggerBuild and handleGitPushWebhook) and returns its id plus the
// progress/finish funcs for Builder.Deploy. source distinguishes the two
// callers in the resulting history row. Falls back to an empty id,
// SlogProgress, and a no-op finish, logged but not returned as an error,
// if deployRecorder is unconfigured or minting/saving the row fails: a
// history-tracking failure must never block a build that would otherwise
// succeed.
func (rt *Router) beginBuildDeployAttempt(ctx context.Context, req deploy.Request, source string) (id string, progress func(build.ProgressEvent), finish func(deployErr error)) {
	noop := func(error) {}
	fallback := build.SlogProgress(rt.logger)

	id, err := store.NewDeployAttemptID()
	if err != nil {
		rt.logger.Error("api: trigger build: mint deploy attempt id failed", slog.String("error", err.Error()))
		return "", fallback, noop
	}

	// Start before SaveDeployAttempt: the row is queryable the instant
	// SaveDeployAttempt returns, so a racing GET must never see id before
	// Start has registered it. Finish below cleans up on a failed save.
	if rt.deployRecorder != nil {
		rt.deployRecorder.Start(id)
	}

	image := req.ImageRepo + ":" + req.CommitSHA
	if err := rt.deployAttempts.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: id, ServiceName: req.ServiceName, Image: image,
		CommitSHA: req.CommitSHA, Source: source,
		Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(),
	}); err != nil {
		rt.logger.Error("api: trigger build: save deploy attempt failed", slog.String("attempt_id", id), slog.String("error", err.Error()))
		if rt.deployRecorder != nil {
			rt.deployRecorder.Finish(ctx, id)
		}
		return "", fallback, noop
	}

	finish = func(deployErr error) {
		finishCtx := context.Background() // survives past r.Context() the same way webhook.Handler.beginDeployAttempt's own finish func does
		if rt.deployRecorder != nil {
			rt.deployRecorder.Finish(finishCtx, id)
		}
		status := store.DeployAttemptStatusSucceeded
		errMsg := ""
		if deployErr != nil {
			status = store.DeployAttemptStatusFailed
			errMsg = deployErr.Error()
		}
		if err := rt.deployAttempts.FinishDeployAttempt(finishCtx, id, status, time.Now(), errMsg); err != nil {
			rt.logger.Error("api: trigger build: finish deploy attempt failed", slog.String("attempt_id", id), slog.String("error", err.Error()))
			return
		}
		// Fires only after FinishDeployAttempt persists the terminal status.
		if rt.deployNotifier != nil {
			rt.deployNotifier.Dispatch(finishCtx, resourceIDForApp(req.ServiceName), alerting.DeployOutcome{
				AppName: req.ServiceName, Image: image, Succeeded: deployErr == nil, Error: errMsg,
			})
		}
	}

	if rt.deployRecorder == nil {
		return id, fallback, finish
	}
	return id, rt.deployRecorder.Progress(id), finish
}

// deployAttemptResource is the wire shape for one deploy attempt.
type deployAttemptResource struct {
	ID          string     `json:"id"`
	ServiceName string     `json:"service_name"`
	Image       string     `json:"image"`
	CommitSHA   string     `json:"commit_sha,omitempty"`
	Source      string     `json:"source,omitempty"`
	Status      string     `json:"status"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	Error       string     `json:"error,omitempty"`
}

func toDeployAttemptResource(a store.DeployAttempt) deployAttemptResource {
	return deployAttemptResource{
		ID:          a.ID,
		ServiceName: a.ServiceName,
		Image:       a.Image,
		CommitSHA:   a.CommitSHA,
		Source:      a.Source,
		Status:      a.Status,
		StartedAt:   a.StartedAt,
		FinishedAt:  a.FinishedAt,
		Error:       a.Error,
	}
}

// handleListDeployAttempts handles GET /api/v1/apps/{name}/deploy-attempts:
// real, row-per-attempt deploy history, newest first, for a frontend
// list to render (status, image, timestamps) with a link to each
// attempt's log viewer and, for a past successful attempt, a rollback
// action (the existing POST .../deploys endpoint, given that attempt's
// own Image).
func (rt *Router) handleListDeployAttempts(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	_, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: list deploy attempts: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	attempts, err := rt.deployAttempts.ListDeployAttempts(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: list deploy attempts failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]deployAttemptResource, 0, len(attempts))
	for _, a := range attempts {
		out = append(out, toDeployAttemptResource(a))
	}
	writeJSON(w, http.StatusOK, out)
}

// sseLogEvent is the exact JSON shape
// web/src/hooks/useDeployLogStream.ts's parseEventPayload prefers:
// { "line": string, "stream": "stdout" | "stderr" }. Named SSE events
// are never used, matching that hook's own doc comment ("everything
// arrives as the default message event").
type sseLogEvent struct {
	Line   string `json:"line"`
	Stream string `json:"stream"`
}

// handleDeployLogStream handles
// GET /api/v1/apps/{name}/deploys/{deployId}/logs: an SSE stream of one
// deploy attempt's build/log output, live from rt.deployRecorder if still
// running or replayed from rt.deployLogStore if already finished. 501 if
// the relevant store isn't configured. Neither branch returns after its
// burst: both hold the connection open until the client disconnects, so
// a reconnecting EventSource never replays and duplicates what it
// already rendered.
func (rt *Router) handleDeployLogStream(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	deployID := r.PathValue("deployId")

	_, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: deploy log stream: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	attempt, err := rt.deployAttempts.GetDeployAttempt(r.Context(), deployID)
	if errors.Is(err, store.ErrDeployAttemptNotFound) {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: deploy log stream: load attempt failed", slog.String("error", err.Error()), slog.String("deploy_id", deployID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Attempt IDs are globally unique but the route is app-scoped: a
	// mismatch here is treated as not-found, not a cross-app leak.
	if attempt.ServiceName != name {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return
	}

	if attempt.FinishedAt != nil {
		rt.serveFinishedDeployLog(r.Context(), w, deployID)
		return
	}
	rt.serveLiveDeployLog(w, r, deployID)
}

// serveFinishedDeployLog writes attemptID's full persisted log as one SSE
// burst, then holds the connection open (writing nothing further) until
// ctx is done, rather than returning immediately: an EventSource
// reconnects automatically when the server closes the connection, which
// would replay this same burst and duplicate it onto whatever the client
// already rendered (onmessage only ever appends).
func (rt *Router) serveFinishedDeployLog(ctx context.Context, w http.ResponseWriter, deployID string) {
	if rt.deployLogStore == nil {
		writeError(w, http.StatusNotImplemented, "deploy log storage is not configured on this control plane")
		return
	}
	entries, err := rt.deployLogStore.QueryDeployLog(ctx, deployID)
	if err != nil {
		rt.logger.Error("api: deploy log stream: query persisted log failed", slog.String("error", err.Error()), slog.String("deploy_id", deployID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	flusher, ok := startSSE(w)
	if !ok {
		rt.logger.Error("api: deploy log stream: response writer does not support flushing", slog.String("deploy_id", deployID))
		return
	}

	// A zero-body WriteHeader+Flush never reaches the browser through
	// Vite's dev proxy (Node's http-proxy only forwards headers on the
	// underlying response's first write()), leaving EventSource.onopen
	// stuck pending. This SSE comment line (ignored by EventSource, never
	// reaching parseEventPayload) guarantees a body byte crosses the wire
	// immediately, in dev and production alike.
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	for _, e := range entries {
		writeSSEEvent(w, sseLogEvent{Line: e.Message, Stream: e.Stream})
	}
	flusher.Flush()

	<-ctx.Done()
}

// serveLiveDeployLog serves deployID's in-progress log: Snapshot's
// lines-so-far replay, then a live tail until the attempt finishes or
// the client disconnects. Falls back to serveFinishedDeployLog's
// persisted-replay behavior if the recorder has no record of this
// attempt (see handleDeployLogStream's own doc comment for when that
// happens).
func (rt *Router) serveLiveDeployLog(w http.ResponseWriter, r *http.Request, deployID string) {
	if rt.deployRecorder == nil {
		writeError(w, http.StatusNotImplemented, "live deploy log streaming is not configured on this control plane")
		return
	}

	lines, live, unsubscribe, ok := rt.deployRecorder.Snapshot(deployID)
	if !ok {
		rt.serveFinishedDeployLog(r.Context(), w, deployID)
		return
	}
	defer unsubscribe()

	flusher, ok := startSSE(w)
	if !ok {
		rt.logger.Error("api: deploy log stream: response writer does not support flushing", slog.String("deploy_id", deployID))
		return
	}

	// Same zero-body-flush gap serveFinishedDeployLog already fixed:
	// write one byte so the browser confirms the connection is open.
	_, _ = fmt.Fprint(w, ": connected\n\n")
	for _, ev := range lines {
		writeSSEEvent(w, sseLogEvent{Line: ev.Line, Stream: ev.Stream})
	}
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, chOpen := <-live:
			if !chOpen {
				// Recorder.Finish closed this channel: the attempt ended
				// and every line was already delivered above. Stay open
				// for the same reconnect-avoidance reason as
				// serveFinishedDeployLog.
				<-r.Context().Done()
				return
			}
			writeSSEEvent(w, sseLogEvent{Line: ev.Line, Stream: ev.Stream})
			flusher.Flush()
		}
	}
}

// startSSE writes the SSE response headers and returns the response's
// http.Flusher. ok is false if w doesn't implement http.Flusher (every
// real net/http ResponseWriter does; this only guards against an
// unusual middleware wrapper that doesn't forward it).
func startSSE(w http.ResponseWriter) (flusher http.Flusher, ok bool) {
	flusher, ok = w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	return flusher, true
}

// writeSSEEvent writes ev as one default-typed SSE message (no `event:`
// field, matching web/src/hooks/useDeployLogStream.ts's own doc comment
// that named events are never used). A JSON marshal failure here can
// only mean a caller bug (sseLogEvent has no field that can fail to
// marshal), so it's logged and skipped rather than propagated: one
// unmarshalable line must not tear down an otherwise-healthy stream.
func writeSSEEvent(w http.ResponseWriter, ev sseLogEvent) {
	data, err := json.Marshal(ev)
	if err != nil {
		slog.Default().Error("api: marshal deploy log event failed", slog.String("error", err.Error()))
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}
