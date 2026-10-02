package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// defaultDeployApprovalTTL is how long a pending deploy approval stays
// decidable before expireIfStale treats it as expired. Overridable via
// APP_DEPLOY_APPROVAL_TTL (cmd/levelrail/main.go, WithDeployApprovalTTL),
// the project's "no hardcoded thresholds" rule.
const defaultDeployApprovalTTL = 24 * time.Hour

func (rt *Router) effectiveDeployApprovalTTL() time.Duration {
	if rt.deployApprovalTTL > 0 {
		return rt.deployApprovalTTL
	}
	return defaultDeployApprovalTTL
}

// deployApprovalResource is the wire shape for a deploy approval.
type deployApprovalResource struct {
	ID                string `json:"id"`
	ServiceName       string `json:"service_name"`
	SourceServiceName string `json:"source_service_name,omitempty"`
	EnvironmentID     string `json:"environment_id"`
	Action            string `json:"action"`
	Image             string `json:"image"`
	Status            string `json:"status"`
	RequestedByType   string `json:"requested_by_type"`
	RequestedBy       string `json:"requested_by"`
	RequestedByName   string `json:"requested_by_name"`
	ApprovedByType    string `json:"approved_by_type,omitempty"`
	ApprovedBy        string `json:"approved_by,omitempty"`
	ApprovedByName    string `json:"approved_by_name,omitempty"`
	Reason            string `json:"reason,omitempty"`
	CreatedAt         string `json:"created_at"`
	ExpiresAt         string `json:"expires_at"`
	DecidedAt         string `json:"decided_at,omitempty"`
	FreezeOverride    string `json:"freeze_override,omitempty"`
	Pull              bool   `json:"pull,omitempty"`
	IncludeEnv        bool   `json:"include_env,omitempty"`
}

func toDeployApprovalResource(a store.DeployApproval) deployApprovalResource {
	return deployApprovalResource{
		ID: a.ID, ServiceName: a.ServiceName, SourceServiceName: a.SourceServiceName,
		EnvironmentID: a.EnvironmentID, Action: a.Action, Image: a.Image, Status: a.Status,
		RequestedByType: a.RequestedByType, RequestedBy: a.RequestedBy, RequestedByName: a.RequestedByName,
		ApprovedByType: a.ApprovedByType, ApprovedBy: a.ApprovedBy, ApprovedByName: a.ApprovedByName,
		Reason: a.Reason, CreatedAt: a.CreatedAt, ExpiresAt: a.ExpiresAt, DecidedAt: a.DecidedAt,
		FreezeOverride: a.FreezeOverride, Pull: a.Pull, IncludeEnv: a.IncludeEnv,
	}
}

// currentActor resolves r's authenticated principal as the same (type,
// id, name) triple requireAbilityDecided/recordAudit already resolve for
// every gated request: a session's user ID and DisplayName, or a bearer
// token's own ID and Name. deploy_approvals.go uses this to record who
// requested/decided an approval, and to compare "is the decider the
// same actor as the requester" against the identical
// PrincipalTypeUser/PrincipalTypeToken vocabulary iam_policy.go's own
// policies already key on.
func (rt *Router) currentActor(r *http.Request) (actorType, actorID, actorName string, ok bool) {
	if userID, sessOK := rt.currentSessionUserID(r); sessOK {
		user, err := rt.auth.GetUserByID(r.Context(), userID)
		if err != nil {
			return "", "", "", false
		}
		return store.PrincipalTypeUser, userID, user.DisplayName, true
	}
	token, tokOK := bearerToken(r)
	if !tokOK {
		return "", "", "", false
	}
	rec, err := rt.tokens.GetAPITokenByHash(r.Context(), hashToken(token))
	if err != nil {
		return "", "", "", false
	}
	return store.PrincipalTypeToken, rec.ID, rec.Name, true
}

// deployApprovalOptions are the request options an approval applies later.
type deployApprovalOptions struct {
	freezeOverride string
	pull           bool
	includeEnv     bool
	promoteEnv     string
}

// requestDeployApproval creates and saves a pending deploy_approvals row
// gating action against env, writing its own 401/500 response and
// returning ok=false on failure, the same "writes its own failure
// response" contract requireEnvironmentConfirmation (environments.go)
// already established. Called by handleTriggerDeploy (deploys.go) and
// handlePromoteApp (promote.go) once each has already confirmed env is
// protected and the caller passed confirm: true.
func (rt *Router) requestDeployApproval(w http.ResponseWriter, r *http.Request, env store.Environment, serviceName, sourceServiceName, action, image string, opts deployApprovalOptions) (deployApprovalResource, bool) {
	actorType, actorID, actorName, ok := rt.currentActor(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return deployApprovalResource{}, false
	}
	id, err := store.NewDeployApprovalID()
	if err != nil {
		rt.internalError(w, "api: request deploy approval: mint id failed", err)
		return deployApprovalResource{}, false
	}
	now := time.Now().UTC()
	a := store.DeployApproval{
		ID: id, ServiceName: serviceName, SourceServiceName: sourceServiceName,
		EnvironmentID: env.ID, Action: action, Image: image,
		Status:          store.DeployApprovalStatusPending,
		RequestedByType: actorType, RequestedBy: actorID, RequestedByName: actorName,
		CreatedAt:      store.FormatAuditTime(now),
		ExpiresAt:      store.FormatAuditTime(now.Add(rt.effectiveDeployApprovalTTL())),
		FreezeOverride: opts.freezeOverride, Pull: opts.pull, IncludeEnv: opts.includeEnv,
		PromoteEnv: opts.promoteEnv,
	}
	if err := rt.deployApprovals.SaveDeployApproval(r.Context(), a); err != nil {
		rt.internalError(w, "api: request deploy approval failed", err, slog.String("service", serviceName))
		return deployApprovalResource{}, false
	}
	rt.notifyApprovalRequested(r.Context(), a)
	return toDeployApprovalResource(a), true
}

// expireIfStale reports a's status as DeployApprovalStatusExpired,
// persisting that transition first, when a is still pending but past its
// ExpiresAt: the lazy sweep every read/decide path below runs before
// acting on a row, so "an expired request never proceeds" holds even
// between RunDeployApprovalExpirySweep's own ticks. Any other status is
// returned unchanged.
func (rt *Router) expireIfStale(ctx context.Context, a store.DeployApproval) store.DeployApproval {
	if a.Status != store.DeployApprovalStatusPending {
		return a
	}
	// ExpiresAt/CreatedAt are formatted with store.FormatAuditTime's fixed-
	// width RFC3339Nano layout, so comparing "now" formatted the same way
	// is a correct plain string comparison, the identical reasoning
	// AuditEntry's own before-cursor filter already relies on.
	now := store.FormatAuditTime(time.Now())
	if now < a.ExpiresAt {
		return a
	}
	ok, err := rt.deployApprovals.DecideDeployApproval(ctx, a.ID, store.DeployApprovalStatusExpired, "", "", "", "", now)
	if err != nil {
		rt.logger.Warn("api: expire deploy approval failed", slog.String("error", err.Error()), slog.String("id", a.ID))
		return a
	}
	if ok {
		a.Status = store.DeployApprovalStatusExpired
		a.DecidedAt = now
	}
	return a
}

type deployApprovalListResponse struct {
	Approvals []deployApprovalResource `json:"approvals"`
}

// handleListDeployApprovals handles
// GET /api/v1/deploy-approvals?status=&service=: status defaults to
// "pending" (the queue an approver actually needs to see), pass
// status=all to see every decided request too.
func (rt *Router) handleListDeployApprovals(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = store.DeployApprovalStatusPending
	}
	if status == "all" {
		status = ""
	}
	service := r.URL.Query().Get("service")

	approvals, err := rt.deployApprovals.ListDeployApprovals(r.Context(), status, service)
	if err != nil {
		rt.internalError(w, "api: list deploy approvals failed", err)
		return
	}
	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: list deploy approvals: visibility", err)
		return
	}
	out := make([]deployApprovalResource, 0, len(approvals))
	for _, a := range approvals {
		if !canSee(a.ServiceName) {
			continue
		}
		if status == store.DeployApprovalStatusPending {
			a = rt.expireIfStale(r.Context(), a)
			if a.Status != store.DeployApprovalStatusPending {
				continue
			}
		}
		out = append(out, toDeployApprovalResource(a))
	}
	writeJSON(w, http.StatusOK, deployApprovalListResponse{Approvals: out})
}

// handleGetDeployApproval handles GET /api/v1/deploy-approvals/{id}.
func (rt *Router) handleGetDeployApproval(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, err := rt.deployApprovals.GetDeployApproval(r.Context(), id)
	if errors.Is(err, store.ErrDeployApprovalNotFound) {
		writeError(w, http.StatusNotFound, "deploy approval not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get deploy approval failed", err, slog.String("id", id))
		return
	}
	a = rt.expireIfStale(r.Context(), a)
	writeJSON(w, http.StatusOK, toDeployApprovalResource(a))
}

type rejectDeployApprovalRequest struct {
	Reason string `json:"reason,omitempty"`
}

// deployApprovalDecisionError is a stable, client-safe reason a
// decide-path couldn't proceed: distinct from a 500, this is always the
// caller's own request being invalid given the approval's current
// state, mirroring unknownRoleError/unknownAbilityError's own "name the
// specific problem" shape. status is the HTTP status an *http.Request
// caller maps it to (writeApprovalDecideError); the chat-interaction
// webhook handlers (chat_interactions.go) read only reason, since a
// Slack/Discord response has no HTTP-status-to-the-original-client
// semantics to preserve.
type deployApprovalDecisionError struct {
	status int
	reason string
}

func (e *deployApprovalDecisionError) Error() string { return e.reason }

// writeApprovalDecideError maps err to an HTTP response: a
// *deployApprovalDecisionError writes its own stable status/reason,
// anything else is an unexpected failure logged via internalError. The
// one error-to-response translation both handleApproveDeployApproval
// and handleRejectDeployApproval use for every error
// loadDecidableApprovalAs/applyAndDecideApproval/rejectApproval can
// return.
func (rt *Router) writeApprovalDecideError(w http.ResponseWriter, context, id string, err error) {
	var de *deployApprovalDecisionError
	if errors.As(err, &de) {
		writeError(w, de.status, de.reason)
		return
	}
	rt.internalError(w, context, err, slog.String("id", id))
}

// loadDecidableApprovalAs loads id, expires it if stale, and reports
// whether it's still decidable (status pending) by a decider distinct
// from its own requester, identified by deciderType/deciderID rather
// than resolved from an *http.Request: the shared core both
// loadDecidableApproval (a session/token caller, via rt.currentActor)
// and the chat-interaction webhook handlers (chat_interactions.go, a
// signature-verified Slack/Discord button click with no session or
// token of its own, see chatApprovalActorType) call, so the dashboard
// and chat approval paths can never diverge on what "still decidable"
// means.
func (rt *Router) loadDecidableApprovalAs(ctx context.Context, id, deciderType, deciderID string) (store.DeployApproval, error) {
	a, err := rt.deployApprovals.GetDeployApproval(ctx, id)
	if errors.Is(err, store.ErrDeployApprovalNotFound) {
		return store.DeployApproval{}, &deployApprovalDecisionError{http.StatusNotFound, "deploy approval not found"}
	}
	if err != nil {
		return store.DeployApproval{}, fmt.Errorf("api: load deploy approval failed: %w", err)
	}
	a = rt.expireIfStale(ctx, a)
	if a.Status != store.DeployApprovalStatusPending {
		return store.DeployApproval{}, &deployApprovalDecisionError{http.StatusConflict, "deploy approval " + id + " is no longer pending (status: " + a.Status + ")"}
	}
	if deciderType == a.RequestedByType && deciderID == a.RequestedBy {
		return store.DeployApproval{}, &deployApprovalDecisionError{http.StatusForbidden, "the same user or token that requested this deploy cannot approve or reject it; a different privileged user must decide"}
	}
	return a, nil
}

// loadDecidableApproval is loadDecidableApprovalAs for a session/token
// caller: resolves the decider from r first (rt.currentActor), writing
// its own 401/404/409/403/500 response and returning ok=false on any
// failure, the same "writes its own failure response" contract
// requestDeployApproval above establishes.
func (rt *Router) loadDecidableApproval(w http.ResponseWriter, r *http.Request, id string) (store.DeployApproval, bool) {
	deciderType, deciderID, _, ok := rt.currentActor(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return store.DeployApproval{}, false
	}
	a, err := rt.loadDecidableApprovalAs(r.Context(), id, deciderType, deciderID)
	if err != nil {
		rt.writeApprovalDecideError(w, "api: load deploy approval failed", id, err)
		return store.DeployApproval{}, false
	}
	return a, true
}

// handleApproveDeployApproval handles
// POST /api/v1/deploy-approvals/{id}/approve: AbilityDeploy-gated
// (routes.go), same tier as triggering the deploy itself would have
// needed, plus loadDecidableApproval's own same-actor rejection above.
// Approving actually runs applyAndDecideApproval below, the exact same
// function the chat-interaction webhook path (chat_interactions.go)
// calls for a verified Slack/Discord button click: the two never
// diverge on what "approved" actually does.
func (rt *Router) handleApproveDeployApproval(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, ok := rt.loadDecidableApproval(w, r, id)
	if !ok {
		return
	}

	deciderType, deciderID, deciderName, _ := rt.currentActor(r)
	resp, err := rt.applyAndDecideApproval(r.Context(), a, deciderType, deciderID, deciderName)
	if err != nil {
		rt.writeApprovalDecideError(w, "api: approve deploy approval failed", id, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// applyAndDecideApproval runs a's gated action through the exact path
// an unprotected deploy/promote already uses (executeConfirmedDeploy,
// deploys.go; setDesiredImage+recordInstantDeployAttempt, promote.go),
// then records the decision. Shared by handleApproveDeployApproval and
// the chat-interaction webhook path (chat_interactions.go), so a
// dashboard click and a verified Slack/Discord click can never apply an
// approval differently.
func (rt *Router) applyAndDecideApproval(ctx context.Context, a store.DeployApproval, deciderType, deciderID, deciderName string) (deployApprovalDecisionResponse, error) {
	svc, err := rt.apps.GetDesiredService(ctx, a.ServiceName)
	if errors.Is(err, store.ErrServiceNotFound) {
		return deployApprovalDecisionResponse{}, &deployApprovalDecisionError{http.StatusConflict, "app " + a.ServiceName + " no longer exists"}
	}
	if err != nil {
		return deployApprovalDecisionResponse{}, fmt.Errorf("api: approve deploy approval: load app failed: %w", err)
	}

	var updated store.DesiredService
	switch a.Action {
	case store.DeployApprovalActionPromote:
		target := *svc
		if a.IncludeEnv {
			if err := applyPromoteEnvSnapshot(&target, a.PromoteEnv); err != nil {
				return deployApprovalDecisionResponse{}, fmt.Errorf("api: approve deploy approval: read env snapshot failed: %w", err)
			}
		}
		updated, err = rt.setDesiredImage(ctx, target, a.Image)
		if err == nil {
			rt.recordInstantDeployAttempt(ctx, updated, a.Image, store.DeployAttemptSourcePromote)
			rt.nudgeReconciler()
		}
	default:
		note, gateErr := rt.approvalFreezeNote(ctx, a)
		if gateErr != nil {
			return deployApprovalDecisionResponse{}, gateErr
		}
		updated, err = rt.executeConfirmedDeploy(ctx, *svc, a.Image, confirmedDeployOptions{pull: a.Pull, reason: note})
	}
	if err != nil && a.Pull && a.Action != store.DeployApprovalActionPromote {
		rt.logger.Error("api: approve deploy approval: fresh pull failed", slog.String("error", err.Error()), slog.String("id", a.ID))
		return deployApprovalDecisionResponse{}, &deployApprovalDecisionError{http.StatusBadGateway, "could not resolve the image from its registry; the approval is still pending, retry later or reject it and request again without pull"}
	}
	if err != nil {
		return deployApprovalDecisionResponse{}, fmt.Errorf("api: approve deploy approval: apply failed: %w", err)
	}

	decidedAt := store.FormatAuditTime(time.Now())
	if _, err := rt.deployApprovals.DecideDeployApproval(ctx, a.ID, store.DeployApprovalStatusApproved, deciderType, deciderID, deciderName, "", decidedAt); err != nil {
		rt.logger.Warn("api: record deploy approval decision failed", slog.String("error", err.Error()), slog.String("id", a.ID))
	}

	final, err := rt.deployApprovals.GetDeployApproval(ctx, a.ID)
	if err != nil {
		return deployApprovalDecisionResponse{}, fmt.Errorf("api: approve deploy approval: reload failed: %w", err)
	}
	return deployApprovalDecisionResponse{Approval: toDeployApprovalResource(final), App: toAppResource(updated)}, nil
}

// approvalFreezeNote refuses to apply an approved deploy during a
// freeze unless the original request carried an override, returning a
// *deployApprovalDecisionError (423 for an HTTP caller) rather than
// writing a response directly: see applyAndDecideApproval's own doc
// comment for why this needs to be callable from both the HTTP handler
// and the chat-interaction webhook path.
func (rt *Router) approvalFreezeNote(ctx context.Context, a store.DeployApproval) (string, error) {
	if rt.deploySafety == nil {
		return "", nil
	}
	status, err := deploy.CheckFreeze(ctx, rt.deploySafety, a.ServiceName, time.Now())
	if err != nil {
		return "", fmt.Errorf("api: approve deploy approval: check freeze failed: %w", err)
	}
	if !status.Frozen {
		return "", nil
	}
	if a.FreezeOverride == "" {
		return "", &deployApprovalDecisionError{http.StatusLocked, (&deploy.FrozenError{Status: status}).Error() + "; this request did not override the freeze, reject it and request again with override_freeze and override_reason"}
	}
	return a.FreezeOverride, nil
}

// deployApprovalDecisionResponse is POST .../approve's response: the
// decided approval, plus the app as it now stands (its desired image
// already pointed at the approved tag; the reconcile that actually
// converges a running container to it is still asynchronous, the same
// "202-shaped" caveat handleTriggerDeploy's own doc comment carries).
type deployApprovalDecisionResponse struct {
	Approval deployApprovalResource `json:"approval"`
	App      appResource            `json:"app"`
}

// handleRejectDeployApproval handles
// POST /api/v1/deploy-approvals/{id}/reject: records the decision and
// leaves the app's desired state untouched, so a rejected request never
// proceeds through reconcile.
func (rt *Router) handleRejectDeployApproval(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, ok := rt.loadDecidableApproval(w, r, id)
	if !ok {
		return
	}

	var req rejectDeployApprovalRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	deciderType, deciderID, deciderName, _ := rt.currentActor(r)
	resource, err := rt.rejectApproval(r.Context(), id, deciderType, deciderID, deciderName, req.Reason)
	if err != nil {
		rt.writeApprovalDecideError(w, "api: reject deploy approval failed", id, err)
		return
	}
	writeJSON(w, http.StatusOK, resource)
}

// rejectApproval records id as rejected, attributed to
// deciderType/deciderID/deciderName, and reloads it: the same function
// handleRejectDeployApproval (a session/token caller, already past
// loadDecidableApproval) and the chat-interaction webhook path
// (chat_interactions.go, already past its own signature check and
// loadDecidableApprovalAs) both call, matching applyAndDecideApproval's
// own "one shared function, two callers" shape for approve.
func (rt *Router) rejectApproval(ctx context.Context, id, deciderType, deciderID, deciderName, reason string) (deployApprovalResource, error) {
	decidedAt := store.FormatAuditTime(time.Now())
	ok, err := rt.deployApprovals.DecideDeployApproval(ctx, id, store.DeployApprovalStatusRejected, deciderType, deciderID, deciderName, reason, decidedAt)
	if err != nil {
		return deployApprovalResource{}, fmt.Errorf("api: reject deploy approval failed: %w", err)
	}
	if !ok {
		return deployApprovalResource{}, &deployApprovalDecisionError{http.StatusConflict, "deploy approval " + id + " is no longer pending"}
	}

	final, err := rt.deployApprovals.GetDeployApproval(ctx, id)
	if err != nil {
		return deployApprovalResource{}, fmt.Errorf("api: reject deploy approval: reload failed: %w", err)
	}
	return toDeployApprovalResource(final), nil
}

// RunDeployApprovalExpirySweep calls expireIfStale-style cleanup across
// every pending approval on interval until ctx is done, the same ticker
// shape RunAuditLogSweeper (audit_retention.go) already establishes. The
// lazy per-request expiry in expireIfStale already guarantees a stale
// request never proceeds; this sweep only matters for a pending row no
// one ever looks at again, so its own status reflects reality in the UI
// without waiting for a read that may never come.
func (rt *Router) RunDeployApprovalExpirySweep(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			n, err := rt.sweepExpiredDeployApprovals(ctx)
			if err != nil {
				rt.logger.Warn("api: deploy approval expiry sweep tick failed", slog.String("error", err.Error()))
			} else if n > 0 {
				rt.logger.Info("api: expired stale deploy approvals", slog.Int("count", n))
			}
		}
	}
}

func (rt *Router) sweepExpiredDeployApprovals(ctx context.Context) (int, error) {
	pending, err := rt.deployApprovals.ListDeployApprovals(ctx, store.DeployApprovalStatusPending, "")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, a := range pending {
		before := a.Status
		after := rt.expireIfStale(ctx, a)
		if before == store.DeployApprovalStatusPending && after.Status == store.DeployApprovalStatusExpired {
			n++
		}
	}
	return n, nil
}
