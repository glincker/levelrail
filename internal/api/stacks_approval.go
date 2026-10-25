package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Approval design: a stack with require_approval stores the validated, planned
// and rendered revision as pending_approval and applies nothing. Secrets are
// never stored, so the approver supplies them in the approve body. Approval
// applies the stored documents under the lease after re-checking the plan
// hash, and the requester can never approve their own request.

const (
	auditStackApprovalRequested = "stack.approval_requested"
	auditStackApprove           = "stack.approve"
	auditStackReject            = "stack.reject"

	defaultStackApprovalTTL = 24 * time.Hour
)

func stackApprovalTTL() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("APP_STACK_APPROVAL_TTL")); err == nil && d > 0 {
		return d
	}
	return defaultStackApprovalTTL
}

// stackApprovalStore is the extra store surface approvals need.
type stackApprovalStore interface {
	SetPendingStackRevisionMeta(ctx context.Context, stackID string, number int, meta []byte) error
}

// stackPendingMeta holds the non-secret apply options kept with a pending revision.
type stackPendingMeta struct {
	Prune           bool `json:"prune,omitempty"`
	AllowMassPrune  bool `json:"allow_mass_prune,omitempty"`
	Adopt           bool `json:"adopt,omitempty"`
	NoDeploy        bool `json:"no_deploy,omitempty"`
	ContinueOnError bool `json:"continue_on_error,omitempty"`
}

type stackApproveRequest struct {
	Secrets          map[string]string `json:"secrets,omitempty"`
	ExpectedPlanHash string            `json:"expected_plan_hash,omitempty"`
}

type stackRejectRequest struct {
	Reason string `json:"reason,omitempty"`
}

type stackRejectResponse struct {
	Stack    string               `json:"stack"`
	Revision stackRevisionSummary `json:"revision"`
	Reason   string               `json:"reason,omitempty"`
}

func approvalExpiry(v store.StackRevisionRecord) time.Time {
	return v.CreatedAt.Add(stackApprovalTTL())
}

func approvalExpired(v store.StackRevisionRecord, now time.Time) bool {
	return v.Status == store.StackRevPendingApproval && !now.Before(approvalExpiry(v))
}

func expiredResult() []byte {
	raw, _ := json.Marshal(map[string]string{"error": "approval expired before anyone approved it"})
	return raw
}

// expireStackApprovals marks stale pending revisions failed; reads call it so
// no background job is needed.
func (rt *Router) expireStackApprovals(ctx context.Context, ss StackStore, s store.StackRecord) {
	revs, err := ss.ListStackRevisions(ctx, s.ID, stackHistoryLimit())
	if err != nil {
		return
	}
	now := time.Now()
	for _, v := range revs {
		if approvalExpired(v, now) {
			_ = ss.SetStackRevisionStatus(context.WithoutCancel(ctx), s.ID, v.Number, store.StackRevFailed, expiredResult(), now)
		}
	}
}

// requestStackApproval validates and plans, then stores a pending revision.
func (rt *Router) requestStackApproval(w http.ResponseWriter, r *http.Request, ss StackStore, run stackRun, res []*iac.Resource, opts iac.Options) {
	ctx := r.Context()
	aps, ok := ss.(stackApprovalStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "approvals are not available on this store")
		return
	}
	stack, req := run.stack, run.req
	project := rt.projectName(ctx, stack.ProjectID)
	lctx, err := rt.stackLedgerContext(ctx, ss, stack.ID, stack.Name, project, res, req)
	if err != nil {
		rt.writeStackError(w, run.action, err)
		return
	}
	st, err := iac.Load(lctx, rt.iacDoer(r), res, opts)
	if err != nil {
		rt.internalError(w, "api: stack approval load failed", err)
		return
	}
	plan := iac.PlanFor(st, res, opts)
	if keys := iac.LedgerConflicts(plan); len(keys) > 0 {
		rt.writeStackError(w, run.action, fmt.Errorf("%w: %v", iac.ErrLedgerConflict, keys))
		return
	}
	if iac.MassPruneBlocked(plan) {
		rt.writeStackError(w, run.action, fmt.Errorf("%w: it exceeds the mass-prune guard; plan to review, then pass allow_mass_prune", iac.ErrPruneNotAllowed))
		return
	}
	if req.ExpectedPlanHash != "" && req.ExpectedPlanHash != plan.Hash {
		rt.writeStackError(w, run.action, iac.ErrPlanChanged)
		return
	}
	caller := rt.stackCaller(r)
	rendered, _ := json.Marshal(renderStackFiles(req.Files, req.Vars))
	planJSON, _ := json.Marshal(plan)
	rev, err := ss.AppendStackRevision(ctx, store.NewStackRevision{
		StackID: stack.ID, Status: store.StackRevPendingApproval, PlanHash: plan.Hash, PlanJSON: planJSON, RenderedDocs: rendered,
		RollbackOf: run.rollbackOf, IdempotencyKey: req.IdempotencyKey, Actor: caller.name, TokenID: caller.tokenID(), RequesterUserID: caller.userID,
		GitRepo: req.GitRepo, GitRef: req.GitRef, GitSHA: req.GitSHA, CIRunURL: req.CIRunURL, Secrets: req.Secrets,
	}, time.Now())
	if err != nil {
		rt.writeStackError(w, run.action, err)
		return
	}
	if rev.Replayed {
		s := summarizeRevision(rev)
		writeJSON(w, http.StatusOK, stackApplyResponse{Stack: stack.Name, Revision: &s, Replayed: true})
		return
	}
	meta, _ := json.Marshal(stackPendingMeta{Prune: req.Prune, AllowMassPrune: req.AllowMassPrune, Adopt: req.Adopt, NoDeploy: req.NoDeploy, ContinueOnError: req.ContinueOnError})
	if err := aps.SetPendingStackRevisionMeta(ctx, stack.ID, rev.Number, meta); err != nil {
		rt.writeStackError(w, run.action, err)
		return
	}
	rt.auditStack(r, auditStackApprovalRequested, map[string]any{
		"stack": stack.Name, "revision": rev.Number, "plan_hash": plan.Hash, "git_sha": req.GitSHA, "rollback_of": run.rollbackOf,
	})
	s := summarizeRevision(rev)
	writeJSON(w, http.StatusAccepted, stackApplyResponse{
		Stack: stack.Name, Revision: &s, Plan: &plan, PendingApproval: true, ExpiresAt: fmtStackTime(approvalExpiry(rev)),
		Warnings: append(run.warnings, "approval required: nothing was applied; an administrator other than the requester must approve this revision and supply any secrets"),
	})
}

// startStackRevision opens the revision an apply runs under: a new applying row,
// or the pending row moved to applying (the store refuses a second approve).
func (rt *Router) startStackRevision(ctx context.Context, w http.ResponseWriter, ss StackStore, run stackRun, caller stackCaller, plan iac.Plan) (store.StackRevisionRecord, bool) {
	stack, req := run.stack, run.req
	if run.pending != nil {
		if err := ss.SetStackRevisionStatus(ctx, stack.ID, run.pending.Number, store.StackRevApplying, nil, time.Now()); err != nil {
			rt.writeStackError(w, run.action, err)
			return store.StackRevisionRecord{}, false
		}
		rev := *run.pending
		rev.Status = store.StackRevApplying
		return rev, true
	}
	rendered, _ := json.Marshal(renderStackFiles(req.Files, req.Vars))
	planJSON, _ := json.Marshal(plan)
	rev, err := ss.AppendStackRevision(ctx, store.NewStackRevision{
		StackID: stack.ID, Status: store.StackRevApplying, PlanHash: plan.Hash, PlanJSON: planJSON, RenderedDocs: rendered,
		RollbackOf: run.rollbackOf, IdempotencyKey: req.IdempotencyKey, Actor: caller.name, TokenID: caller.tokenID(), RequesterUserID: caller.userID,
		GitRepo: req.GitRepo, GitRef: req.GitRef, GitSHA: req.GitSHA, CIRunURL: req.CIRunURL, Secrets: req.Secrets,
	}, time.Now())
	if err != nil {
		rt.writeStackError(w, run.action, err)
		return store.StackRevisionRecord{}, false
	}
	return rev, true
}

// pendingStillApprovable re-reads the revision under the lease so a reject or
// an expiry that landed while waiting wins.
func (rt *Router) pendingStillApprovable(ctx context.Context, w http.ResponseWriter, ss StackStore, stack store.StackRecord, n int) bool {
	cur, err := ss.GetStackRevision(ctx, stack.ID, n)
	if err != nil {
		rt.writeStackError(w, "approve", err)
		return false
	}
	return rt.checkApprovable(ctx, w, ss, stack, cur)
}

func (rt *Router) checkApprovable(ctx context.Context, w http.ResponseWriter, ss StackStore, stack store.StackRecord, v store.StackRevisionRecord) bool {
	if v.Status == store.StackRevFailed && bytes.Equal(v.ResultJSON, expiredResult()) {
		writeError(w, http.StatusGone, fmt.Sprintf("the approval for revision %d expired; request a new apply", v.Number))
		return false
	}
	if v.Status != store.StackRevPendingApproval {
		writeError(w, http.StatusConflict, fmt.Sprintf("revision %d is %s, not awaiting approval", v.Number, v.Status))
		return false
	}
	if approvalExpired(v, time.Now()) {
		_ = ss.SetStackRevisionStatus(context.WithoutCancel(ctx), stack.ID, v.Number, store.StackRevFailed, expiredResult(), time.Now())
		writeError(w, http.StatusGone, fmt.Sprintf("the approval for revision %d expired; request a new apply", v.Number))
		return false
	}
	return true
}

func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxIaCBodyBytes)).Decode(out)
	if err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}

func (rt *Router) handleStackApprove(w http.ResponseWriter, r *http.Request) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return
	}
	s, ok := rt.loadStack(w, r, ss)
	if !ok {
		return
	}
	n, ok := revisionNumber(w, r)
	if !ok {
		return
	}
	var body stackApproveRequest
	if !decodeOptionalJSON(w, r, &body) {
		return
	}
	rev, err := ss.GetStackRevision(r.Context(), s.ID, n)
	if err != nil {
		rt.writeStackError(w, "approve", err)
		return
	}
	caller := rt.stackCaller(r)
	if caller.isCI {
		writeError(w, http.StatusForbidden, "a CI token cannot approve a stack revision")
		return
	}
	if !rt.checkApprovable(r.Context(), w, ss, s, rev) {
		return
	}
	if !distinctApprover(caller, rev) {
		writeError(w, http.StatusForbidden, "the requester cannot approve their own revision; another administrator must approve it")
		return
	}
	if body.ExpectedPlanHash != "" && body.ExpectedPlanHash != rev.PlanHash {
		writeError(w, http.StatusConflict, "the plan changed since it was reviewed; re-plan and request approval again")
		return
	}
	files, err := decodeRenderedFiles(rev.RenderedDocs)
	if err != nil {
		rt.internalError(w, "api: stack revision documents unreadable", err)
		return
	}
	var meta stackPendingMeta
	if len(rev.ResultJSON) > 0 {
		_ = json.Unmarshal(rev.ResultJSON, &meta)
	}
	req := stackRequest{
		Files: files, Secrets: body.Secrets, Prune: meta.Prune, AllowMassPrune: meta.AllowMassPrune, Adopt: meta.Adopt,
		NoDeploy: meta.NoDeploy, ContinueOnError: meta.ContinueOnError, ExpectedPlanHash: rev.PlanHash,
		GitRepo: rev.GitRepo, GitRef: rev.GitRef, GitSHA: rev.GitSHA, CIRunURL: rev.CIRunURL,
	}
	run := stackRun{stack: s, req: req, rollbackOf: rev.RollbackOf, action: auditStackApprove, pending: &rev}
	if rev.RollbackOf > 0 {
		run.warnings = []string{rollbackWarning}
	}
	rt.runStack(w, r, ss, run)
}

// distinctApprover compares stable user ids, never display or token names, and
// refuses when either side has no known user (an ownerless token or an old row).
func distinctApprover(caller stackCaller, rev store.StackRevisionRecord) bool {
	if caller.userID == "" || rev.RequesterUserID == "" {
		return false
	}
	if caller.userID == rev.RequesterUserID {
		return false
	}
	return caller.tokenID() == "" || caller.tokenID() != rev.TokenID
}

func (rt *Router) handleStackReject(w http.ResponseWriter, r *http.Request) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return
	}
	s, ok := rt.loadStack(w, r, ss)
	if !ok {
		return
	}
	n, ok := revisionNumber(w, r)
	if !ok {
		return
	}
	var body stackRejectRequest
	if !decodeOptionalJSON(w, r, &body) {
		return
	}
	rev, err := ss.GetStackRevision(r.Context(), s.ID, n)
	if err != nil {
		rt.writeStackError(w, "reject", err)
		return
	}
	if rev.Status != store.StackRevPendingApproval {
		writeError(w, http.StatusConflict, fmt.Sprintf("revision %d is %s, not awaiting approval", n, rev.Status))
		return
	}
	caller := rt.stackCaller(r)
	result, _ := json.Marshal(map[string]string{"error": "rejected", "rejected_by": caller.name, "reason": body.Reason})
	if err := ss.SetStackRevisionStatus(r.Context(), s.ID, n, store.StackRevFailed, result, time.Now()); err != nil {
		rt.writeStackError(w, "reject", err)
		return
	}
	rev.Status = store.StackRevFailed
	now := time.Now()
	rev.FinishedAt = &now
	rt.auditStack(r, auditStackReject, map[string]any{"stack": s.Name, "revision": n, "plan_hash": rev.PlanHash, "requested_by": rev.Actor, "reason": body.Reason})
	writeJSON(w, http.StatusOK, stackRejectResponse{Stack: s.Name, Revision: summarizeRevision(rev), Reason: body.Reason})
}
