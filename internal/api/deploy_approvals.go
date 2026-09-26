package api

import (
	"context"
	"encoding/json"
	"errors"
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
// decide-path handler couldn't proceed: distinct from a 500, this is
// always the caller's own request being invalid given the approval's
// current state, mirroring unknownRoleError/unknownAbilityError's own
// "name the specific problem" shape.
type deployApprovalDecisionError struct{ reason string }

func (e *deployApprovalDecisionError) Error() string { return e.reason }

// loadDecidableApproval loads id, expires it if stale, and reports
// whether it's still decidable (status pending) by a decider distinct
// from its own requester. Writes its own 404/409/500 response and
// returns ok=false otherwise, the same "writes its own failure
// response" contract requestDeployApproval above establishes.
func (rt *Router) loadDecidableApproval(w http.ResponseWriter, r *http.Request, id string) (store.DeployApproval, bool) {
	a, err := rt.deployApprovals.GetDeployApproval(r.Context(), id)
	if errors.Is(err, store.ErrDeployApprovalNotFound) {
		writeError(w, http.StatusNotFound, "deploy approval not found")
		return store.DeployApproval{}, false
	}
	if err != nil {
		rt.internalError(w, "api: load deploy approval failed", err, slog.String("id", id))
		return store.DeployApproval{}, false
	}
	a = rt.expireIfStale(r.Context(), a)
	if a.Status != store.DeployApprovalStatusPending {
		writeError(w, http.StatusConflict, "deploy approval "+id+" is no longer pending (status: "+a.Status+")")
		return store.DeployApproval{}, false
	}

	deciderType, deciderID, _, ok := rt.currentActor(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return store.DeployApproval{}, false
	}
	if deciderType == a.RequestedByType && deciderID == a.RequestedBy {
		writeError(w, http.StatusForbidden, "the same user or token that requested this deploy cannot approve or reject it; a different privileged user must decide")
		return store.DeployApproval{}, false
	}
	return a, true
}

// handleApproveDeployApproval handles
// POST /api/v1/deploy-approvals/{id}/approve: AbilityDeploy-gated
// (routes.go), same tier as triggering the deploy itself would have
// needed, plus loadDecidableApproval's own same-actor rejection above.
// Approving actually runs the gated action through the exact path an
// unprotected deploy/promote already uses (executeConfirmedDeploy,
// deploys.go; setDesiredImage+recordInstantDeployAttempt, promote.go),
// so an approved request converges through reconcile identically to any
// other deploy.
func (rt *Router) handleApproveDeployApproval(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, ok := rt.loadDecidableApproval(w, r, id)
	if !ok {
		return
	}

	svc, err := rt.apps.GetDesiredService(r.Context(), a.ServiceName)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusConflict, "app "+a.ServiceName+" no longer exists")
		return
	}
	if err != nil {
		rt.internalError(w, "api: approve deploy approval: load app failed", err, slog.String("id", id))
		return
	}

	var updated store.DesiredService
	switch a.Action {
	case store.DeployApprovalActionPromote:
		target := *svc
		if a.IncludeEnv {
			if err := applyPromoteEnvSnapshot(&target, a.PromoteEnv); err != nil {
				rt.internalError(w, "api: approve deploy approval: read env snapshot failed", err, slog.String("id", id))
				return
			}
		}
		updated, err = rt.setDesiredImage(r.Context(), target, a.Image)
		if err == nil {
			rt.recordInstantDeployAttempt(r.Context(), updated, a.Image, store.DeployAttemptSourcePromote)
			rt.nudgeReconciler()
		}
	default:
		note, ok := rt.approvalFreezeGate(w, r, a)
		if !ok {
			return
		}
		updated, err = rt.executeConfirmedDeploy(r.Context(), *svc, a.Image, confirmedDeployOptions{pull: a.Pull, reason: note})
	}
	if err != nil && a.Pull && a.Action != store.DeployApprovalActionPromote {
		rt.logger.Error("api: approve deploy approval: fresh pull failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusBadGateway, "could not resolve the image from its registry; the approval is still pending, retry later or reject it and request again without pull")
		return
	}
	if err != nil {
		rt.internalError(w, "api: approve deploy approval: apply failed", err, slog.String("id", id))
		return
	}

	deciderType, deciderID, deciderName, _ := rt.currentActor(r)
	decidedAt := store.FormatAuditTime(time.Now())
	if _, err := rt.deployApprovals.DecideDeployApproval(r.Context(), id, store.DeployApprovalStatusApproved, deciderType, deciderID, deciderName, "", decidedAt); err != nil {
		rt.logger.Warn("api: record deploy approval decision failed", slog.String("error", err.Error()), slog.String("id", id))
	}

	final, err := rt.deployApprovals.GetDeployApproval(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: approve deploy approval: reload failed", err, slog.String("id", id))
		return
	}
	writeJSON(w, http.StatusOK, deployApprovalDecisionResponse{
		Approval: toDeployApprovalResource(final),
		App:      toAppResource(updated),
	})
}

// approvalFreezeGate refuses to apply an approved deploy during a freeze
// unless the original request carried an override.
func (rt *Router) approvalFreezeGate(w http.ResponseWriter, r *http.Request, a store.DeployApproval) (string, bool) {
	if rt.deploySafety == nil {
		return "", true
	}
	status, err := deploy.CheckFreeze(r.Context(), rt.deploySafety, a.ServiceName, time.Now())
	if err != nil {
		rt.internalError(w, "api: approve deploy approval: check freeze failed", err, slog.String("id", a.ID))
		return "", false
	}
	if !status.Frozen {
		return "", true
	}
	if a.FreezeOverride == "" {
		writeError(w, http.StatusLocked, (&deploy.FrozenError{Status: status}).Error()+"; this request did not override the freeze, reject it and request again with override_freeze and override_reason")
		return "", false
	}
	return a.FreezeOverride, true
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
	decidedAt := store.FormatAuditTime(time.Now())
	ok, err := rt.deployApprovals.DecideDeployApproval(r.Context(), id, store.DeployApprovalStatusRejected, deciderType, deciderID, deciderName, req.Reason, decidedAt)
	if err != nil {
		rt.internalError(w, "api: reject deploy approval failed", err, slog.String("id", id))
		return
	}
	if !ok {
		writeError(w, http.StatusConflict, "deploy approval "+id+" is no longer pending")
		return
	}

	final, err := rt.deployApprovals.GetDeployApproval(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: reject deploy approval: reload failed", err, slog.String("id", id))
		return
	}
	writeJSON(w, http.StatusOK, toDeployApprovalResource(final))
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
