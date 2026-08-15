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

// beginBuildDeployAttempt mints and saves a deploy_attempts row for
// req (handleTriggerBuild's manual git-source build path, the other of
// the two real trigger paths that runs an actual build, alongside
// internal/webhook's own identical beginDeployAttempt), and returns the
// progress func to pass to Builder.Deploy plus a finish func the caller
// must invoke exactly once with Deploy's error, success or failure.
//
// If rt.deployRecorder is nil (WithDeployRecorder not configured), this
// falls back to build.SlogProgress, this package's pre-existing
// behavior, but still records the attempt row via rt.deployAttempts
// (always available: it's part of the core Store interface, not an
// optional plug-in, see DeployAttemptStore's own doc comment), just with
// no persisted log content, the same "degrade the log, not the history"
// choice deployAttemptEnabled makes one layer down in internal/webhook.
// If minting or saving the row itself fails, this degrades further to a
// no-op finish and SlogProgress, logged but not returned as this
// request's own error: a history-tracking failure must never block a
// build that would otherwise succeed.
//
// image mirrors internal/deploy's own deployDockerfile tag construction
// (ImageRepo + ":" + CommitSHA), computed here before the build runs so
// a failed build's history row still shows what tag it was trying to
// produce.
func (rt *Router) beginBuildDeployAttempt(ctx context.Context, req deploy.Request) (progress func(build.ProgressEvent), finish func(deployErr error)) {
	noop := func(error) {}
	fallback := build.SlogProgress(rt.logger)

	id, err := store.NewDeployAttemptID()
	if err != nil {
		rt.logger.Error("api: trigger build: mint deploy attempt id failed", slog.String("error", err.Error()))
		return fallback, noop
	}

	image := req.ImageRepo + ":" + req.CommitSHA
	if err := rt.deployAttempts.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: id, ServiceName: req.ServiceName, Image: image,
		Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(),
	}); err != nil {
		rt.logger.Error("api: trigger build: save deploy attempt failed", slog.String("attempt_id", id), slog.String("error", err.Error()))
		return fallback, noop
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
		// Deploy-outcome notification (wave-2 roadmap item #5), the
		// build-triggered counterpart to recordPlainDeployAttempt's own
		// dispatch and internal/webhook.Handler.beginDeployAttempt's
		// finish closure: fires only once FinishDeployAttempt has
		// persisted the attempt's terminal status, never for the
		// in-progress "running" status this closure's caller (Deploy)
		// hasn't returned from yet.
		if rt.deployNotifier != nil {
			rt.deployNotifier.Dispatch(finishCtx, resourceIDForApp(req.ServiceName), alerting.DeployOutcome{
				AppName: req.ServiceName, Image: image, Succeeded: deployErr == nil, Error: errMsg,
			})
		}
	}

	if rt.deployRecorder == nil {
		return fallback, finish
	}
	rt.deployRecorder.Start(id)
	return rt.deployRecorder.Progress(id), finish
}

// deployAttemptResource is the wire shape for one deploy attempt.
type deployAttemptResource struct {
	ID          string     `json:"id"`
	ServiceName string     `json:"service_name"`
	Image       string     `json:"image"`
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
// GET /api/v1/apps/{name}/deploys/{deployId}/logs: an SSE stream serving
// one deploy attempt's build/log output, live or replayed depending on
// whether the attempt has finished by the time this request lands. Both
// cases are real requirements, not just the live one: deploys in this
// product are frequently unattended (a git push webhook), so a user
// typically opens this view *after* a deploy already finished, and needs
// the exact same output a live viewer would have seen, not an empty
// "nothing here" response just because they weren't watching in real
// time.
//
//   - Finished attempt (attempt.FinishedAt set): served entirely from
//     the persisted store (rt.deployLogStore.QueryDeployLog) in one
//     shot, then the response stays open (writing nothing further)
//     until the client disconnects. 501 if no DeployLogStore is
//     configured (WithDeployLogStore).
//   - In-progress attempt: served from rt.deployRecorder.Snapshot, which
//     atomically returns every line so far plus a live channel for
//     everything after (see that method's own doc comment for the
//     race-free guarantee). The response stays open, flushing each new
//     line as it arrives, until the client disconnects
//     (r.Context().Done()); once the live channel itself closes (Finish
//     was called, meaning the attempt ended), nothing more will ever
//     arrive on it, but the response still doesn't return, for the same
//     reason as the finished-attempt case below. 501 if no Recorder is
//     configured (WithDeployRecorder). If the recorder has no record of
//     this attempt at all (Snapshot's ok=false: e.g. the control plane
//     restarted mid-deploy and lost its in-memory state, a known,
//     accepted gap this task does not build crash-recovery for), this
//     falls back to the persisted store the same way a finished attempt
//     would, so a viewer at least sees whatever was flushed before the
//     restart instead of an empty response.
//
// Neither branch ever returns right after its one real burst of data:
// both hold the connection open (blocking on the client disconnecting)
// instead. This was found by this task's own live browser verification,
// not designed upfront: a real EventSource client (the frontend's
// useDeployLogStream.ts) reconnects automatically the moment the server
// closes the connection, the same behavior that hook's own doc comment
// cites as a reason SSE was chosen over WebSockets in the first place.
// An earlier version of this handler returned immediately after its
// burst, which closed the response; the browser observed that as a
// dropped connection, reconnected within its default retry interval,
// landed back in the very same branch, and got the *entire* persisted
// log again, appended onto whatever was already rendered (that hook's
// onmessage handler has no concept of "this is a replay, clear first,"
// it only ever appends). Watched live, the log view's line count kept
// growing and the same lines kept repeating every few seconds for as
// long as the tab stayed open. Holding the connection open once there's
// nothing more to send avoids ever triggering that reconnect at all.
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
	// Prevents an attempt ID from one app being viewed through another
	// app's URL: attempt IDs are opaque and globally unique
	// (store.NewDeployAttemptID), but the route is app-scoped, so a
	// mismatch here means the caller has the wrong app name for this
	// attempt, treated as "not found" rather than a silent cross-app
	// leak.
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

// serveFinishedDeployLog writes attemptID's full persisted log as one
// SSE burst, then holds the connection open (writing nothing further)
// until ctx is done (the client disconnects, e.g. navigates away),
// rather than returning immediately after the burst.
//
// This blocking-instead-of-closing shape is deliberate, found by this
// task's own live browser verification, not a hypothetical: a real
// EventSource client reconnects automatically after the server closes
// the connection (the same behavior useDeployLogStream.ts's own doc
// comment cites as a reason SSE was chosen over WebSockets in the first
// place), and this handler previously returned right after the burst,
// which closed the HTTP response. The browser observed that as a
// dropped connection and reconnected within its default retry interval,
// landing back in this same finished-attempt branch, which replayed the
// entire persisted log again and appended it to whatever the client had
// already rendered (useDeployLogStream.ts's onmessage handler has no
// concept of "this is a replay, clear first," it only ever appends).
// The result, confirmed live: the log view's line count kept growing
// and the same lines kept repeating, roughly every 3 seconds, for as
// long as the tab stayed open. Keeping the connection open after a
// finished attempt's one real burst means the server never closes it,
// so the browser's EventSource has nothing to reconnect from and stays
// in its normal "open" state indefinitely, exactly matching what an
// already-finished, nothing-more-to-say stream should look like from a
// client's point of view.
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

	// Write one harmless SSE comment line immediately, before the entries
	// loop below (which, for the zero-log-lines case this function
	// exists to handle, writes nothing at all). SSE comment lines start
	// with ':' and are never dispatched as an event (EventSource ignores
	// them outright, so parseEventPayload in
	// web/src/hooks/useDeployLogStream.ts never sees this), but writing
	// it guarantees at least one body byte crosses the wire right away.
	//
	// Found by this task's own live browser + network-panel verification:
	// against Vite's dev proxy, a zero-entries response left the SSE
	// request showing no status code and no response headers at all in
	// Chrome's network inspector, for as long as the tab stayed open, and
	// the frontend's connection badge stayed on "Connecting..." forever,
	// because WriteHeader+Flush alone (with zero body bytes written) is
	// not enough to make Node's http-proxy (which Vite's dev server uses
	// for server.proxy) forward the response headers to the browser: it
	// only does that on the underlying response's first write() call,
	// which a header-only, zero-body flush never triggers. Confirmed via
	// the same live setup against the real production path (the control
	// plane's own embedded net/http server serving the built frontend
	// directly, no proxy in front) that this is dev-proxy-only: there,
	// EventSource.onopen fires immediately off WriteHeader+Flush alone
	// and the badge correctly reads "Live" within a couple seconds, with
	// or without this comment line. Writing it unconditionally makes the
	// zero-entries case behave the same way in both environments instead
	// of depending on proxy-specific flushing behavior.
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
				// The attempt finished: Recorder.Finish closed this
				// channel. Every line was already delivered live above
				// (or is in the persisted store if one was dropped, see
				// Progress's own doc comment on the bounded subscriber
				// buffer), so there is nothing left to send, but the
				// connection stays open rather than returning here for
				// the identical reason serveFinishedDeployLog does: an
				// EventSource client would otherwise reconnect, land in
				// serveFinishedDeployLog's now-finished branch, and get
				// a full duplicate replay of everything it just watched
				// live.
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
