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
	"github.com/GLINCKER/levelrail/internal/failure"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/supplychain"
	"github.com/GLINCKER/levelrail/kit/untrusted"
)

// This file is the real deploy-attempt history and log surface: a
// row per real trigger call (deploys.go's
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
// callers in the resulting history row. svc is the service's desired
// state as of this trigger, snapshotted onto the row for
// GET .../deploys/compare: a build changes Image, not Env/Port/Domains/
// Resources, so the caller's already-loaded pre-build DesiredService is
// exactly what this attempt is building against. Falls back to an empty
// id, SlogProgress, and a no-op finish, logged but not returned as an
// error, if deployRecorder is unconfigured or minting/saving the row
// fails: a history-tracking failure must never block a build that would
// otherwise succeed.
//
// setCommit re-points the row (and the deploy notification) at the
// commit a caller only learns after its checkout resolves, so the row's
// image always matches the tag actually built: a rollback deploys that
// image by name. Callers that already hold a full SHA (the webhook
// paths) never call it.
func (rt *Router) beginBuildDeployAttempt(ctx context.Context, req deploy.Request, svc store.DesiredService, source, detectedFramework string) (id string, progress func(build.ProgressEvent), finish func(deployErr error), setCommit func(ctx context.Context, commit string)) {
	noop := func(error) {}
	noopCommit := func(context.Context, string) {}
	fallback := build.SlogProgress(rt.logger)

	id, adopted := adoptedAttemptID(ctx)
	if !adopted {
		var err error
		if id, err = store.NewDeployAttemptID(); err != nil {
			rt.logger.Error("api: trigger build: mint deploy attempt id failed", slog.String("error", err.Error()))
			return "", fallback, noop, noopCommit
		}
	}

	// Start before SaveDeployAttempt: the row is queryable the instant
	// SaveDeployAttempt returns, so a racing GET must never see id before
	// Start has registered it. Finish below cleans up on a failed save.
	if rt.deployRecorder != nil {
		rt.deployRecorder.Start(id)
	}

	image := req.ImageRepo + ":" + req.CommitSHA
	var seq int64
	if req.Order != nil {
		seq = req.Order.Sequence
	}
	rt.cancels.Register(id)
	defer releaseStartMark(ctx)
	if !adopted {
		if err := rt.deployAttempts.SaveDeployAttempt(ctx, store.DeployAttempt{
			ID: id, ServiceName: req.ServiceName, Image: image,
			CommitSHA: req.CommitSHA, Source: source,
			Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(),
			Snapshot:          store.NewDeployAttemptSnapshot(svc),
			DetectedFramework: detectedFramework,
			Sequence:          seq,
			Branch:            commitMetaFrom(ctx).Branch,
			CommitMessage:     commitMetaFrom(ctx).Message,
			Author:            commitMetaFrom(ctx).Author,
			RerunOf:           rerunOfFrom(ctx),
		}); err != nil {
			rt.logger.Error("api: trigger build: save deploy attempt failed", slog.String("attempt_id", id), slog.String("error", err.Error()))
			rt.cancels.Release(id)
			if rt.deployRecorder != nil {
				rt.deployRecorder.Finish(ctx, id)
			}
			return "", fallback, noop, noopCommit
		}
	}

	setCommit = func(setCtx context.Context, commit string) {
		if commit == "" || commit == req.CommitSHA {
			return
		}
		image = req.ImageRepo + ":" + commit
		if err := rt.deployAttempts.SetDeployAttemptCommit(setCtx, id, image, commit); err != nil {
			rt.logger.Error("api: trigger build: update deploy attempt commit failed", slog.String("attempt_id", id), slog.String("error", err.Error()))
		}
	}

	finish = func(deployErr error) {
		finishCtx := context.Background() // survives past r.Context() the same way webhook.Handler.beginDeployAttempt's own finish func does
		defer rt.drainAfterFinish(finishCtx, id)
		if rt.deployRecorder != nil {
			rt.deployRecorder.Finish(finishCtx, id)
		}
		if rt.finishCanceled(finishCtx, id, deployErr) {
			return
		}
		if rt.finishSuperseded(finishCtx, id, deployErr) {
			return
		}
		status := store.DeployAttemptStatusSucceeded
		errMsg := ""
		if deployErr != nil {
			status = store.DeployAttemptStatusFailed
			errMsg = deployErrorMessage(deployErr)
		}
		if err := rt.deployAttempts.FinishDeployAttempt(finishCtx, id, status, time.Now(), errMsg); err != nil {
			rt.logger.Error("api: trigger build: finish deploy attempt failed", slog.String("attempt_id", id), slog.String("error", err.Error()))
			return
		}
		// Fires only after FinishDeployAttempt persists the terminal status.
		if rt.deployNotifier != nil {
			outcome := alerting.DeployOutcome{
				AppName: req.ServiceName, Image: image, Succeeded: deployErr == nil, Error: errMsg, AttemptID: id,
			}
			if deployErr == nil {
				outcome.ReleaseNotes = rt.releaseNotesForDeploy(finishCtx, req.ServiceName, "")
			}
			rt.deployNotifier.Dispatch(finishCtx, resourceIDForApp(req.ServiceName), outcome)
		}
	}

	inner := fallback
	if rt.deployRecorder != nil {
		inner = rt.deployRecorder.Progress(id)
	}
	return id, rt.recordCacheWarning(id, inner), finish, setCommit
}

// recordCacheWarning stores a build cache warning on the attempt row while
// passing every event through.
func (rt *Router) recordCacheWarning(id string, next func(build.ProgressEvent)) func(build.ProgressEvent) {
	setter, ok := rt.deployAttempts.(cacheWarningSetter)
	if !ok {
		return next
	}
	return func(ev build.ProgressEvent) {
		if ev.CacheWarning != "" {
			if err := setter.SetDeployAttemptCacheWarning(context.Background(), id, ev.CacheWarning); err != nil {
				rt.logger.Warn("api: record cache warning failed", slog.String("attempt_id", id), slog.String("error", err.Error()))
			}
		}
		next(ev)
	}
}

// emitStep records one named pipeline-phase transition for id (see
// deploylog.Recorder.Step), a no-op if no recorder is configured. Kept
// as a Router method so call sites (builds.go) don't need to nil-check
// rt.deployRecorder themselves.
func (rt *Router) emitStep(id, step, status string) {
	if id == "" {
		return
	}
	if rt.deployRecorder != nil {
		rt.deployRecorder.Step(id, step, status)
	}
	rt.persistStep(id, step, status)
}

// deployAttemptResource is the wire shape for one deploy attempt.
type deployAttemptResource struct {
	ID                string     `json:"id"`
	ServiceName       string     `json:"service_name"`
	Image             string     `json:"image"`
	CommitSHA         string     `json:"commit_sha,omitempty"`
	Source            string     `json:"source,omitempty"`
	Status            string     `json:"status"`
	StartedAt         time.Time  `json:"started_at"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	Error             string     `json:"error,omitempty"`
	DetectedFramework string     `json:"detected_framework,omitempty"`
	// Branch, CommitMessage and Author come from the push payload that
	// triggered a git build; empty for image and dashboard deploys.
	Branch        string `json:"branch,omitempty"`
	CommitMessage string `json:"commit_message,omitempty"`
	Author        string `json:"author,omitempty"`
	// Trigger classifies Source the way the deployments list does.
	Trigger string `json:"trigger,omitempty"`
	// CacheWarning is set when the build carried on without its remote cache.
	CacheWarning string `json:"cache_warning,omitempty"`

	ImageDigest  string `json:"image_digest,omitempty"`
	DigestReason string `json:"digest_reason,omitempty"`
	// Kind is "static" for a container-less static site deploy.
	Kind           string `json:"kind,omitempty"`
	RolloutState   string `json:"rollout_state,omitempty"`
	RunningImageID string `json:"running_image_id,omitempty"`
	Sequence       int64  `json:"sequence,omitempty"`
	Reason         string `json:"reason,omitempty"`

	// QueuedAt, QueuePosition and WaitReason describe a queued deploy (and
	// WaitReason a held one); BlockedBy is the deploy it waits for.
	QueuedAt      *time.Time `json:"queued_at,omitempty"`
	QueuePosition int        `json:"queue_position,omitempty"`
	WaitReason    string     `json:"wait_reason,omitempty"`
	BlockedBy     string     `json:"blocked_by,omitempty"`
	// SupersededBy is the newer deploy that replaced a queued one.
	SupersededBy string `json:"superseded_by,omitempty"`
	// CanceledBy is who canceled the deploy.
	CanceledBy string `json:"canceled_by,omitempty"`
	// RerunOf is the attempt this one retried or rebuilt; ChainPosition is
	// its 1-based place in that chain, set by the list and single endpoints.
	RerunOf       string `json:"rerun_of,omitempty"`
	ChainPosition int    `json:"chain_position,omitempty"`
	// PreviewImageURL is set when a deploy preview thumbnail exists.
	PreviewImageURL string `json:"preview_image_url,omitempty"`
	// DeployURL is this deploy's own immutable address while a retained copy exists.
	DeployURL string `json:"deploy_url,omitempty"`
	// Live is true for the deploy the app runs right now, served at the app's own URL.
	Live bool `json:"live,omitempty"`
	// PublishState is "held" for a finished build the app's publish lock kept off the live site.
	PublishState string `json:"publish_state,omitempty"`
	// SBOMPackages and VulnCounts are set when the build produced an SBOM and it was scanned.
	SBOMPackages *int                `json:"sbom_packages,omitempty"`
	VulnCounts   *supplychain.Counts `json:"vuln_counts,omitempty"`
	// Failure classifies why the deploy failed or is blocked; absent otherwise.
	Failure *failure.Failure `json:"failure,omitempty"`
	// Outcome is in_progress, healthy, failed, canceled, superseded or
	// blocked, on every response that carries an attempt.
	Outcome string `json:"outcome,omitempty"`
	// OutcomeReason is a one-line why behind Outcome.
	OutcomeReason string `json:"outcome_reason,omitempty"`
	// RolloutCrashLoopRestarts is the restart count at which a never-serving
	// rollout counts as failed, read-only so clients apply the same rule.
	RolloutCrashLoopRestarts int `json:"rollout_crash_loop_restarts,omitempty"`
	// Phase is the stored attempt status when the single-deploy endpoint
	// reports a derived Status (building, rolling_out, failed).
	Phase string `json:"phase,omitempty"`
	// Exit is set by the single-deploy endpoint when the release's container
	// stopped before it could serve: how, and its last output.
	Exit *deployExitResource `json:"exit,omitempty"`
}

// applyWait fills the queue and wait fields of res for attempt a.
func (rt *Router) applyWait(ctx context.Context, res *deployAttemptResource, a store.DeployAttempt, app []store.DeployAttempt) {
	if a.Status != store.DeployAttemptStatusQueued && a.Status != store.DeployAttemptStatusHeld {
		return
	}
	w := rt.waitFor(ctx, a, app)
	res.QueuePosition, res.WaitReason, res.BlockedBy = w.Position, w.Reason, w.BlockedBy
}

func toDeployAttemptResource(a store.DeployAttempt) deployAttemptResource {
	kind := ""
	if isStaticAttempt(a) {
		kind = "static"
	}
	return deployAttemptResource{
		Kind:              kind,
		ID:                a.ID,
		ServiceName:       a.ServiceName,
		Image:             a.Image,
		CommitSHA:         a.CommitSHA,
		Source:            a.Source,
		Status:            a.Status,
		StartedAt:         a.StartedAt,
		FinishedAt:        a.FinishedAt,
		Error:             a.Error,
		DetectedFramework: a.DetectedFramework,
		Branch:            a.Branch,
		CommitMessage:     a.CommitMessage,
		Author:            a.Author,
		Trigger:           store.DeploymentTrigger(a.Source, false, 0),
		ImageDigest:       a.ImageDigest,
		DigestReason:      a.DigestReason,
		RolloutState:      a.RolloutState,
		RunningImageID:    a.RunningImageID,
		Sequence:          a.Sequence,
		Reason:            a.Reason,
		QueuedAt:          a.QueuedAt,
		SupersededBy:      a.SupersededBy,
		CanceledBy:        a.CanceledBy,
		RerunOf:           a.RerunOf,
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

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: list deploy attempts: load app failed", err, slog.String("name", name))
		return
	}

	attempts, err := rt.deployAttempts.ListDeployAttempts(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: list deploy attempts failed", err, slog.String("name", name))
		return
	}

	var warnings map[string]string
	if l, ok := rt.deployAttempts.(cacheWarningLister); ok {
		if warnings, err = l.ListDeployAttemptCacheWarnings(r.Context(), name); err != nil {
			rt.logger.Warn("api: list deploy attempt cache warnings failed", slog.String("error", err.Error()), slog.String("name", name))
		}
	}
	previewURLs := rt.previewImageURLs(r.Context(), name)
	ids := make([]string, 0, len(attempts))
	for _, a := range attempts {
		ids = append(ids, a.ID)
	}
	supply := rt.supplyChainRecords(r.Context(), ids)
	deployURLs := rt.deployURLsByAttempt(r.Context(), name)
	held := rt.heldBuildIDs(r.Context(), name)
	conds := rt.rolloutConditions(r.Context(), name)
	out := make([]deployAttemptResource, 0, len(attempts))
	chainPos := rerunChainPositions(attempts)
	liveID := newestSucceededID(attempts)
	for i, a := range attempts {
		res := toDeployAttemptResource(a)
		applyDeployOutcome(&res, a, i == 0, conds)
		if isRedeployOfEarlierImage(a, attempts) {
			res.Trigger = store.DeploymentTriggerRollback
		}
		res.ChainPosition = chainPos(a.ID)
		res.DeployURL = deployURLs[a.ID]
		res.Live = a.ID == liveID && a.Image == svc.Image
		if held[a.ID] && (!res.Live || isStaticAttempt(a)) {
			res.Live = false
			res.PublishState = publishStateHeld
		}
		res.CacheWarning = warnings[a.ID]
		rt.attachFailure(&res, a)
		rt.applyWait(r.Context(), &res, a, attempts)
		res.PreviewImageURL = previewURLs[a.ID]
		if rec, ok := supply[a.ID]; ok {
			n, counts := supplyChainRecordSummary(rec)
			res.SBOMPackages, res.VulnCounts = &n, counts
		}
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, parseDeployAttemptFilter(r).apply(out))
}

// cacheWarningLister and cacheWarningSetter are optional deployAttempts
// capabilities, so stores that predate build cache warnings keep working.
type cacheWarningLister interface {
	ListDeployAttemptCacheWarnings(ctx context.Context, serviceName string) (map[string]string, error)
}

type cacheWarningSetter interface {
	SetDeployAttemptCacheWarning(ctx context.Context, id, warning string) error
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
		rt.internalError(w, "api: deploy log stream: load app failed", err, slog.String("name", name))
		return
	}

	attempt, err := rt.deployAttempts.GetDeployAttempt(r.Context(), deployID)
	if errors.Is(err, store.ErrDeployAttemptNotFound) {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: deploy log stream: load attempt failed", err, slog.String("deploy_id", deployID))
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
		rt.internalError(w, "api: deploy log stream: query persisted log failed", err, slog.String("deploy_id", deployID))
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

	rows := rt.traceRows(ctx, deployID)
	sent := map[string]string{}
	for _, r := range rows {
		sent[r.Key] = stepSignature(r)
	}
	for _, e := range rt.mergeTraceIntoLog(ctx, deployID, entries) {
		writeSSEEvent(w, sseLogEvent{Line: e.Message, Stream: e.Stream})
	}
	flusher.Flush()

	rt.followTraceLog(ctx, w, flusher, deployID, true, sent)
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

	flusher, ok := rt.startLiveSSE(w, r)
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
	sent := map[string]string{}
	if rows := rt.traceRows(r.Context(), deployID); len(rows) > 0 {
		for _, e := range traceLogEntries(deployID, rows) {
			writeSSEEvent(w, sseLogEvent{Line: e.Message, Stream: e.Stream})
		}
		for _, row := range rows {
			sent[row.Key] = stepSignature(row)
		}
	}
	flusher.Flush()

	traceTick := time.NewTicker(durationEnv(EnvTracePollInterval, time.Second))
	defer traceTick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-traceTick.C:
			rt.writeChangedTraceLines(r.Context(), w, flusher, deployID, sent)
		case ev, chOpen := <-live:
			if !chOpen {
				// Recorder.Finish closed this channel: the attempt ended
				// and every line was already delivered above. Keep
				// following trace rows the reconciler adds after the
				// attempt finished, and stay open for the same
				// reconnect-avoidance reason as serveFinishedDeployLog.
				rt.followTraceLog(r.Context(), w, flusher, deployID, true, sent)
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

// deployErrorMessage is stored and served on the deploy record and the error
// carries repo-author build output, so strip control codes, redact and bound it.
func deployErrorMessage(err error) string {
	return untrusted.Sanitize(err.Error(), failureOptions().MaxBytes)
}
