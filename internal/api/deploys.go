package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/distribution/reference"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/deploytrace"
	"github.com/GLINCKER/levelrail/internal/diskguard"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// applicationControllerName mirrors
// internal/reconcile/application.Controller.Name()'s "application/" +
// serviceName convention. Duplicated as a plain string format here
// rather than imported, because importing that package would pull
// internal/docker's whole runtime dependency into internal/api just for
// one string format; if that naming convention ever changes, both call
// sites need updating together.
func applicationControllerName(appName string) string {
	return "application/" + appName
}

type deployTriggerRequest struct {
	Image   string `json:"image"`
	Confirm bool   `json:"confirm,omitempty"`
	// Pull re-resolves the tag against the registry and fails rather than
	// deploying a cached image when the registry is unreachable.
	Pull bool `json:"pull,omitempty"`
	// Rollback refuses an image that was never a previous deploy of this app
	// at trigger time instead of failing later at pull time.
	Rollback bool `json:"rollback,omitempty"`
	// RerunOf is the deploy attempt of this app that this deploy retries.
	RerunOf string `json:"rerun_of,omitempty"`
	freezeOverride
	policyOverride
}

// deployTriggerResult is POST .../deploys and POST .../promote's shared
// response shape. *appResource is embedded anonymously so the common
// case (no protected environment, or one applied immediately) still
// marshals as a flat app object exactly like before this type existed;
// PendingApproval is only ever set instead, when the target environment
// is protected and the request was accepted as a pending approval rather
// than applied (requestDeployApproval, deploy_approvals.go). A nil
// embedded pointer contributes no keys, so the two shapes never overlap
// on the wire.
type deployTriggerResult struct {
	*appResource
	PendingApproval *deployApprovalResource `json:"pending_approval,omitempty"`
}

// handleTriggerDeploy handles POST /api/v1/apps/{name}/deploys. It
// points the app's desired image at a new tag; the application
// controller's next reconcile (once main.go wires one for this app, see
// router.go's package doc comment) is what actually creates a container
// from it. This is exactly the mechanism used for
// rollback ("pointing desired.Image back at an older tag and
// reconciling converges to it the same way any other redeploy does"),
// run forward with a newer tag instead of an older one. The build that
// produces that tag (1.4/1.5) is not this endpoint's job.
//
// If name is tagged with a protected environment, confirm: true is
// required just to be accepted at all (checkEnvironmentProtection,
// environments.go); once accepted, the deploy itself does not apply yet,
// it becomes a pending deploy_approvals row a different, sufficiently
// privileged user must approve (deploy_approvals.go) before this same
// image tag actually reaches executeConfirmedDeploy below.
func (rt *Router) handleTriggerDeploy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	existing, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: trigger deploy: load app failed", err, slog.String("name", name))
		return
	}

	var req deployTriggerRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if req.Image == "" {
		writeError(w, http.StatusBadRequest, "image is required")
		return
	}
	if err := validateImageReference(req.Image); err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeInvalidImageRef, "image: "+err.Error())
		return
	}

	if status, msg := rt.validateRerunOf(r.Context(), name, req.RerunOf); status != 0 {
		writeError(w, status, msg)
		return
	}

	if req.Rollback && !rt.checkRollbackTarget(w, r, *existing, req.Image) {
		return
	}

	rt.applyDeploy(w, r, *existing, req, "")
}

const errCodeInvalidImageRef = "invalid_image_reference"

// validateImageReference rejects an image that Docker could never pull, so the
// mistake surfaces at trigger time instead of as a failed deploy later.
func validateImageReference(image string) error {
	if _, err := reference.ParseNormalizedNamed(image); err != nil {
		return fmt.Errorf("%q is not a valid image reference (expected name[:tag] or name@sha256:digest, repository names are lowercase)", image)
	}
	return nil
}

// applyDeploy runs a deploy request through the freeze gate and the
// protected-environment gate, then applies it. rollbackTo, when set, marks
// the attempt as a rollback to that deploy.
func (rt *Router) applyDeploy(w http.ResponseWriter, r *http.Request, existing store.DesiredService, req deployTriggerRequest, rollbackTo string) {
	name := existing.Name
	note, ok := rt.freezeGate(w, r, name, req.freezeOverride)
	if !ok {
		return
	}
	// Rollbacks skip guardrails: restoring a known state must never be blocked.
	if rollbackTo == "" && (rt.refuseLowDisk(w, diskguard.OpPull) || rt.refuseNodeCritical(r.Context(), w, existing.NodeID)) {
		return
	}
	if rollbackTo == "" {
		candidate := existing
		if req.Image != "" {
			candidate.Image = req.Image
		}
		policyNote, pok := rt.policyGate(w, r, candidate, req.policyOverride)
		if !pok {
			return
		}
		note = strings.TrimSpace(note + " " + policyNote)
	}
	if rollbackTo != "" {
		note = strings.TrimSpace(note + " RollbackTo: " + rollbackTo)
	}

	// Observed live: a rollback issued while a manual build was still
	// running wrote its own desired state immediately, then the build's
	// own deploy finished moments later and silently clobbered it, with
	// no sign to the operator that the rollback never stuck. Same guard
	// handleTriggerBuild already applies against itself (builds.go), now
	// also checked here so a plain image deploy or rollback can't race a
	// build the same way.
	if rt.hasRunningDeployAttempt(r.Context(), name) {
		writeError(w, http.StatusConflict, "a deploy for this app is already running")
		return
	}

	env, protected, ok := rt.checkEnvironmentProtection(r.Context(), w, existing.EnvironmentID, req.Confirm)
	if !ok {
		return
	}
	if protected {
		approval, ok := rt.requestDeployApproval(w, r, env, existing.Name, "", store.DeployApprovalActionDeploy, req.Image, deployApprovalOptions{freezeOverride: note, pull: req.Pull})
		if !ok {
			return
		}
		writeJSON(w, http.StatusAccepted, deployTriggerResult{PendingApproval: &approval})
		return
	}

	updated, err := rt.executeConfirmedDeploy(r.Context(), existing, req.Image, confirmedDeployOptions{pull: req.Pull, reason: note, rerunOf: req.RerunOf})
	if err != nil {
		rt.logger.Error("api: trigger deploy failed", slog.String("error", err.Error()), slog.String("name", name))
		if errors.Is(err, docker.ErrImageNotFound) {
			writeError(w, http.StatusUnprocessableEntity, "image pull failed: "+err.Error())
			return
		}
		if req.Pull {
			writeError(w, http.StatusBadGateway, "could not resolve the image from its registry; retry without pull to use the cached image")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	res := toAppResource(updated)
	writeJSON(w, http.StatusAccepted, deployTriggerResult{appResource: &res})
}

// executeConfirmedDeploy runs the actual desired-state change a deploy
// trigger makes, once any protected-environment gate has already passed
// (either there was none, or a deploy approval for it was just
// approved, see handleApproveDeployApproval in deploy_approvals.go): the
// exact save+record+nudge path internal/alerting's crashloop
// auto-rollback (MaybeAutoRollback) also drives, so a manual deploy, an
// approved one, and an automatic rollback all converge through identical
// code, never three divergent ones.
// confirmedDeployOptions carries a manual deploy's pull and freeze
// override choices into executeConfirmedDeploy.
type confirmedDeployOptions struct {
	pull   bool
	reason string
	// rerunOf is the validated attempt this deploy retries, empty if none.
	rerunOf string
	// source overrides the attempt trigger; empty keeps the image default.
	source string
}

func (rt *Router) executeConfirmedDeploy(ctx context.Context, existing store.DesiredService, image string, opts confirmedDeployOptions) (store.DesiredService, error) {
	// Read before Deploy, which records its own newest attempt.
	recreate := rt.redeployNeedsRecreate(ctx, existing)
	trigger := rt.imageTrigger(ctx, existing, opts.pull, opts.reason)
	trigger.RerunOf = opts.rerunOf
	if opts.source != "" {
		trigger.Source = opts.source
	}
	updated, err := trigger.Deploy(ctx, existing, image)
	if err != nil {
		return store.DesiredService{}, err
	}
	// Same image means the same container name, so an exited container would be
	// restarted with its old env behind the crash backoff. Recreate only when
	// the app is broken or its env changed; a healthy unchanged app is left alone.
	if updated.Image == existing.Image && recreate {
		if err := rt.apps.RestartService(ctx, existing.Name); err != nil {
			return store.DesiredService{}, fmt.Errorf("deploy: recreate %q for redeploy: %w", existing.Name, err)
		}
		if fresh, err := rt.apps.GetDesiredService(ctx, existing.Name); err == nil {
			updated = *fresh
		}
	}
	if rt.deployNotifier != nil {
		rt.deployNotifier.Dispatch(ctx, resourceIDForApp(existing.Name), alerting.DeployOutcome{
			AppName: existing.Name, Image: updated.Image, Succeeded: true,
			ReleaseNotes: rt.releaseNotesForDeploy(ctx, existing.Name, ""),
		})
	}
	return updated, nil
}

// deployTriggerImageStore adapts rt.apps and rt.deployAttempts, two
// separately-typed Router fields, to the single deploy.ImageDeployStore
// surface deploy.TriggerImageDeploy needs: Go interfaces don't compose
// across two independent fields without an adapter like this one.
type deployTriggerImageStore struct{ rt *Router }

func (s deployTriggerImageStore) SaveDesiredService(ctx context.Context, svc store.DesiredService) error {
	return s.rt.apps.SaveDesiredService(ctx, svc)
}

func (s deployTriggerImageStore) SaveDeployAttempt(ctx context.Context, a store.DeployAttempt) error {
	return s.rt.deployAttempts.SaveDeployAttempt(ctx, a)
}

// SaveDeployTraceStep lets deploy.ImageTrigger write its trace through the
// same store the attempt lives in.
func (s deployTriggerImageStore) SaveDeployTraceStep(ctx context.Context, step store.DeployTraceStep) error {
	sink, ok := s.rt.deployAttempts.(deploytrace.Sink)
	if !ok {
		return nil
	}
	return sink.SaveDeployTraceStep(ctx, step)
}

func (s deployTriggerImageStore) FinishDeployAttempt(ctx context.Context, id, status string, finishedAt time.Time, errMsg string) error {
	return s.rt.deployAttempts.FinishDeployAttempt(ctx, id, status, finishedAt, errMsg)
}

// setAutoRollbackRequest is PUT /api/v1/apps/{name}/auto-rollback's body.
type setAutoRollbackRequest struct {
	Enabled bool `json:"enabled"`
}

// autoRollbackSettingResource is both GET and PUT
// /api/v1/apps/{name}/auto-rollback's response.
type autoRollbackSettingResource struct {
	Enabled bool `json:"enabled"`
}

// handleGetAutoRollback handles GET /api/v1/apps/{name}/auto-rollback:
// the current value of store.DesiredService.AutoRollbackOnCrashloop, so
// the dashboard toggle (and "apps auto-rollback status") has something to
// read on load without waiting for a PUT.
func (rt *Router) handleGetAutoRollback(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get auto-rollback: load app failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, autoRollbackSettingResource{Enabled: svc.AutoRollbackOnCrashloop})
}

// handleSetAutoRollback handles PUT /api/v1/apps/{name}/auto-rollback:
// opts name into (or out of) automatic rollback on a crashloop, the
// per-app switch internal/alerting.MaybeAutoRollback checks before
// rolling back a firing KindCrashloop rule's app. Off by default
// (migrations/0106_service_auto_rollback_on_crashloop.sql), matching
// every other opt-in feature toggle in this codebase.
func (rt *Router) handleSetAutoRollback(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setAutoRollbackRequest
	if !decodeReq(w, r, &req) {
		return
	}

	if err := rt.apps.SetServiceAutoRollbackOnCrashloop(r.Context(), name, req.Enabled); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		rt.internalError(w, "api: set auto-rollback failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, autoRollbackSettingResource(req))
}

// validSLOBurnAutoRollbackModes is every store.AutoRollbackSLOBurn* value
// handleSetAutoRollbackSLOBurn accepts.
var validSLOBurnAutoRollbackModes = map[string]bool{
	store.AutoRollbackSLOBurnOff:           true,
	store.AutoRollbackSLOBurnAuto:          true,
	store.AutoRollbackSLOBurnDryRun:        true,
	store.AutoRollbackSLOBurnPauseForHuman: true,
}

// setAutoRollbackSLOBurnRequest is PUT
// /api/v1/apps/{name}/auto-rollback-slo-burn's body.
type setAutoRollbackSLOBurnRequest struct {
	Mode string `json:"mode"`
}

// autoRollbackSLOBurnSettingResource is both GET and PUT
// /api/v1/apps/{name}/auto-rollback-slo-burn's response.
type autoRollbackSLOBurnSettingResource struct {
	Mode string `json:"mode"`
}

// handleGetAutoRollbackSLOBurn handles GET
// /api/v1/apps/{name}/auto-rollback-slo-burn: the current value of
// store.DesiredService.AutoRollbackOnSLOBurn, mirroring
// handleGetAutoRollback's own shape for the crashloop toggle.
func (rt *Router) handleGetAutoRollbackSLOBurn(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get auto-rollback-slo-burn: load app failed", err, slog.String("name", name))
		return
	}
	mode := svc.AutoRollbackOnSLOBurn
	if mode == "" {
		mode = store.AutoRollbackSLOBurnOff
	}
	writeJSON(w, http.StatusOK, autoRollbackSLOBurnSettingResource{Mode: mode})
}

// handleSetAutoRollbackSLOBurn handles PUT
// /api/v1/apps/{name}/auto-rollback-slo-burn: sets name's mode for
// automatic rollback on an SLO burn-rate alert, the per-app switch
// internal/alerting.MaybeAutoRollbackOnSLOBurn checks before acting on a
// firing KindSLOBurn rule's app. "off" by default
// (migrations/0251_service_auto_rollback_on_slo_burn.sql), matching
// AutoRollbackOnCrashloop's own opt-in-by-default shape.
func (rt *Router) handleSetAutoRollbackSLOBurn(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setAutoRollbackSLOBurnRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if !validSLOBurnAutoRollbackModes[req.Mode] {
		writeError(w, http.StatusBadRequest, "mode must be one of: off, auto, dry_run, pause_for_human")
		return
	}

	if err := rt.apps.SetServiceAutoRollbackOnSLOBurn(r.Context(), name, req.Mode); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		rt.internalError(w, "api: set auto-rollback-slo-burn failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, autoRollbackSLOBurnSettingResource(req))
}

// setDesiredImage points existing's image at image and saves it: the
// same mutation handleTriggerDeploy and handlePromoteApp (promote.go)
// both drive, clearing EnvDirty the same way RestartService already
// does for a real redeploy.
func (rt *Router) setDesiredImage(ctx context.Context, existing store.DesiredService, image string) (store.DesiredService, error) {
	updated := existing
	updated.Image = image
	updated.EnvDirty = false
	if err := rt.apps.SaveDesiredService(ctx, updated); err != nil {
		return store.DesiredService{}, err
	}
	return updated, nil
}

// recordPlainDeployAttempt records a deploy_attempts row for the plain
// image-tag path (create, update, and manual redeploy/rollback all share
// it): it deserves history and rollback treatment just like the two
// build-triggering paths, even with no build step to wait on. svc is the
// service's desired state as of this trigger (already carrying image, in
// every real call site), snapshotted onto the row for
// GET .../deploys/compare. See recordInstantDeployAttempt for the
// mechanics.
func (rt *Router) recordPlainDeployAttempt(ctx context.Context, svc store.DesiredService, image string) {
	rt.recordInstantDeployAttempt(ctx, svc, image, store.DeployAttemptSourceImage)
}

// recordInstantDeployAttempt saves and immediately finishes (as
// succeeded) a deploy_attempts row for a trigger with no build step and
// no async work to wait on: the caller's own desired-state write has
// already fully completed by the time this runs. source distinguishes
// the plain image-tag path (recordPlainDeployAttempt) from the
// compose/template fan-out path (apps_compose.go's handleDeployCompose),
// which calls this directly with DeployAttemptSourceCompose once per
// service.
//
// A failure here (minting the ID, or either store call) is logged, not
// returned as the caller's own error: the desired-state write already
// succeeded, so a history-tracking hiccup must never turn an otherwise-
// successful deploy trigger into a client-visible failure.
func (rt *Router) recordInstantDeployAttempt(ctx context.Context, svc store.DesiredService, image, source string) {
	rt.recordResolvedDeployAttempt(ctx, svc, image, source, deploy.ImageResolution{}, nil)
}

// recordResolvedDeployAttempt is recordInstantDeployAttempt carrying the
// image resolution: the digest, how it was obtained and, for a local-only
// image, a short reason so history never shows an unexplained blank. pre
// holds trace steps measured before the attempt had an id (nil for none);
// they are written to the new attempt together with its config step.
func (rt *Router) recordResolvedDeployAttempt(ctx context.Context, svc store.DesiredService, image, source string, res deploy.ImageResolution, pre *deploytrace.Tracer) {
	serviceName := svc.Name
	id, err := store.NewDeployAttemptID()
	if err != nil {
		rt.logger.Error("api: record deploy attempt: mint id failed", slog.String("error", err.Error()), slog.String("name", serviceName))
		return
	}
	digestReason, reason := describeResolution(res)
	now := time.Now()
	if err := rt.deployAttempts.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: id, ServiceName: serviceName, Image: image,
		Source: source,
		Status: store.DeployAttemptStatusRunning, StartedAt: now,
		Snapshot:    store.NewDeployAttemptSnapshot(svc),
		ImageDigest: res.Digest, DigestReason: digestReason, Reason: reason,
	}); err != nil {
		rt.logger.Error("api: record deploy attempt: save failed", slog.String("error", err.Error()), slog.String("attempt_id", id))
		return
	}
	rt.traceInstantDeploy(ctx, id, svc, image, source, res, pre)
	if err := rt.deployAttempts.FinishDeployAttempt(ctx, id, store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		rt.logger.Error("api: record deploy attempt: finish failed", slog.String("error", err.Error()), slog.String("attempt_id", id))
		return
	}
	if rt.deployNotifier != nil && source != store.DeployAttemptSourceRestart && source != store.DeployAttemptSourceStart {
		rt.deployNotifier.Dispatch(ctx, resourceIDForApp(serviceName), alerting.DeployOutcome{
			AppName: serviceName, Image: image, Succeeded: true,
			ReleaseNotes: rt.releaseNotesForDeploy(ctx, serviceName, source),
		})
	}
}

// describeResolution maps an image resolution to the digest reason and the
// short attempt reason to record. A local-only image (no registry digest)
// is reported as LocalImage instead of a blank.
func describeResolution(res deploy.ImageResolution) (digestReason, reason string) {
	digestReason = res.Reason
	switch {
	case res.LocalID != "" && docker.ImageDigestOf(res.Image) == "" && res.Reason != store.DigestReasonLocalBuild:
		return store.DigestReasonLocalImage, "LocalImage: no registry digest, running the local image"
	case res.Reason == store.DigestReasonUnresolved:
		return digestReason, "Unresolved: image not found in a registry or locally"
	}
	return digestReason, ""
}

// handleDeployHistory handles GET /api/v1/apps/{name}/deploys. It
// surfaces the application controller's stored reconcile conditions for
// this app, per the reconciler contract that every reconcile emits a
// status condition with a reason string, stored and shown in the UI.
// This is current reconcile status, not a log of past deploy attempts:
// internal/store.UpsertConditions only ever persists the latest
// condition per (controller, condition type) pair, it has no history
// table. Router.go's package doc comment tracks that as a real gap
// rather than something this handler fakes with an invented log format.
func (rt *Router) handleDeployHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: deploy history: load app failed", err, slog.String("name", name))
		return
	}

	conditions, err := rt.deploys.GetConditions(r.Context(), applicationControllerName(name))
	if err != nil {
		rt.internalError(w, "api: deploy history failed", err, slog.String("name", name))
		return
	}
	// This app's own controller never sees CrossNodeIngress, reported
	// under the ingress controller's own singleton name: see
	// crossNodeIngressAppCondition's doc comment.
	if cond := rt.crossNodeIngressAppCondition(r.Context(), *svc); cond != nil {
		conditions = append(conditions, *cond)
	}
	writeJSON(w, http.StatusOK, rt.markRestartRollingOut(r.Context(), *svc, conditions))
}

// markRestartRollingOut replaces a stored Ready=True while the newest attempt
// is a restart that is not serving yet: that Ready belongs to the old container.
func (rt *Router) markRestartRollingOut(ctx context.Context, svc store.DesiredService, conditions []reconcile.Condition) []reconcile.Condition {
	if rt.deployAttempts == nil || svc.Suspended {
		return conditions
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, svc.Name)
	if err != nil || len(attempts) == 0 {
		return conditions
	}
	newest := attempts[0]
	if newest.Source != store.DeployAttemptSourceRestart || newest.Status != store.DeployAttemptStatusSucceeded || newest.RolloutState == store.RolloutStateServing {
		return conditions
	}
	out := slices.Clone(conditions)
	for i, c := range out {
		if c.Type == reconcile.ConditionTypeReady && c.Status == reconcile.ConditionTrue {
			out[i] = reconcile.Condition{Type: c.Type, Status: reconcile.ConditionFalse, Reason: "RollingOut", Message: "restart in progress: the new container has not passed readiness yet", LastTransitionTime: newest.StartedAt}
		}
	}
	return out
}

// recreateSignals are the facts that make a same-image redeploy recreate the
// container instead of leaving it alone.
type recreateSignals struct {
	containerStopped bool
	latestFailed     bool
	failingReason    string
	envChanged       bool
}

var recreateFailureReasons = map[string]bool{
	deploysafety.ReasonReadinessFailed:       true,
	deploysafety.ReasonExitedDuringReadiness: true,
	deploysafety.ReasonWatchCrashloop:        true,
	"CrashLoopBackOff":                       true,
	"ContainerCrashed":                       true,
}

func (s recreateSignals) needed() bool {
	return s.containerStopped || s.latestFailed || recreateFailureReasons[s.failingReason] || s.envChanged
}

// redeployNeedsRecreate gathers the signals from existing status helpers. The
// env signal is the env_dirty latch; there is no cheaper spec-drift check.
func (rt *Router) redeployNeedsRecreate(ctx context.Context, existing store.DesiredService) bool {
	sig := recreateSignals{envChanged: existing.EnvDirty}
	if st, ok := rt.appRunState(ctx, existing.Name); ok && !st.Running {
		sig.containerStopped = true
	}
	if rt.deploys != nil {
		sig.failingReason, _ = rt.failingReadyCondition(ctx, existing.Name)
	}
	if rt.deployAttempts != nil {
		if list, err := rt.deployAttempts.ListDeployAttempts(ctx, existing.Name); err == nil && len(list) > 0 {
			sig.latestFailed = list[0].Status == store.DeployAttemptStatusFailed
		}
	}
	return sig.needed()
}
