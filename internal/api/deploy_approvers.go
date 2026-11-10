package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// approvalVoteStore records distinct approvers. Optional: a store without it
// keeps the single approver path, which is also every environment's default.
type approvalVoteStore interface {
	RecordDeployApprovalVote(ctx context.Context, v store.DeployApprovalVote) (bool, error)
	ReplaceDeployApprovalVote(ctx context.Context, v, prev store.DeployApprovalVote) (bool, error)
	ListDeployApprovalVotes(ctx context.Context, approvalID string) ([]store.DeployApprovalVote, error)
	EnvironmentRequiredApprovers(ctx context.Context, envID string) (int, error)
}

// environmentApproverSetter is the admin write side of the per environment count.
type environmentApproverSetter interface {
	SetEnvironmentRequiredApprovers(ctx context.Context, envID string, n int, updatedAt string) error
	EnvironmentRequiredApprovers(ctx context.Context, envID string) (int, error)
}

func (rt *Router) approvalVotes() (approvalVoteStore, bool) {
	vs, ok := rt.deployApprovals.(approvalVoteStore)
	return vs, ok
}

func (rt *Router) maxRequiredApprovers(ctx context.Context) int {
	if rt.runtimeSettings == nil {
		return runtimesettings.ClampLimit(runtimesettings.KeyDeployApprovalMaxRequired, 10)
	}
	return rt.runtimeSettings.Limit(ctx, runtimesettings.KeyDeployApprovalMaxRequired)
}

// requiredApprovers is the live count for envID, clamped to [1, runtime max]
// so lowering the max takes effect on approvals already waiting.
func (rt *Router) requiredApprovers(ctx context.Context, vs approvalVoteStore, envID string) (int, error) {
	if envID == "" {
		return 1, nil
	}
	n, err := vs.EnvironmentRequiredApprovers(ctx, envID)
	if err != nil {
		return 0, err
	}
	return min(max(n, 1), max(rt.maxRequiredApprovers(ctx), 1)), nil
}

// A chat channel, system actor or ownerless token is not a person, so it
// cannot be one of N distinct approvers.
var errVoteNeedsPrincipal = &deployApprovalDecisionError{http.StatusForbidden, "this environment needs several distinct approvers; approve as a signed in user or with an API token owned by a user (dashboard or CLI)"}

var errVoterNotAllowed = &deployApprovalDecisionError{http.StatusForbidden, "you no longer hold the deploy ability or access to this app, so your approval cannot count"}

var errStaleApproval = &deployApprovalDecisionError{http.StatusConflict, "this held deploy was superseded or already released; approve the current request instead"}

var errUnpinnedImage = &deployApprovalDecisionError{http.StatusConflict, "this environment needs several approvers and the image tag could not be resolved to a digest; pin the image by digest or promote from a built image"}

var errTagMoved = &deployApprovalDecisionError{http.StatusConflict, "the image tag now points at different content than earlier approvers saw; reject this request and request the deploy again"}

// pinnedImage resolves a tag only deploy or promote to its registry digest so
// every vote approves the same content. Releases and held replays are left as is.
func (rt *Router) pinnedImage(ctx context.Context, a store.DeployApproval) (ref, digest string, err error) {
	if (a.Action != store.DeployApprovalActionDeploy && a.Action != store.DeployApprovalActionPromote) || a.ReleaseID != "" {
		return "", "", nil
	}
	if docker.ImageDigestOf(a.Image) != "" {
		return a.Image, docker.ImageDigestOf(a.Image), nil
	}
	if rt.imageResolver == nil {
		return "", "", errUnpinnedImage
	}
	var svc store.DesiredService
	if got, gerr := rt.apps.GetDesiredService(ctx, a.ServiceName); gerr == nil {
		svc = *got
	}
	res, err := rt.imageResolver.ResolveImage(ctx, a.Image, rt.imageTrigger(ctx, svc, true, "approval").Auth, true)
	if err != nil || res.Digest == "" {
		return "", "", errUnpinnedImage
	}
	return res.Ref, res.Digest, nil
}

// approvalBinding fingerprints what releasing a would run. A vote only counts
// while the fingerprint still matches, so a replaced request starts from zero.
func (rt *Router) approvalBinding(ctx context.Context, a store.DeployApproval, digest string) (string, error) {
	h := sha256.New()
	for _, part := range []string{a.ID, a.Action, a.ServiceName, a.SourceServiceName, a.Image, a.ReleaseID, a.PromoteEnv, a.HeldAttemptID, digest} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	if a.Action == store.DeployApprovalActionReplay {
		at, err := rt.deployAttempts.GetDeployAttempt(ctx, a.HeldAttemptID)
		if err != nil || at == nil || at.Status != store.DeployAttemptStatusAwaitingApproval {
			return "", errStaleApproval
		}
		for _, part := range []string{at.Image, at.CommitSHA, at.HeldRequest} {
			h.Write([]byte(part))
			h.Write([]byte{0})
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// approverKey names a distinct principal: the human when there is one, so
// a user's tokens and sessions all count once.
func approverKey(human, typ, id string) string {
	if human != "" {
		return store.PrincipalTypeUser + ":" + human
	}
	return typ + ":" + id
}

func requesterKey(a store.DeployApproval) string {
	return approverKey(a.RequesterUserID, a.RequestedByType, a.RequestedBy)
}

func (rt *Router) userStillActive(ctx context.Context, userID string) ([]string, bool) {
	if rt.auth == nil {
		return nil, false
	}
	u, err := rt.auth.GetUserByID(ctx, userID)
	if err != nil || u == nil || rt.accountDisabled(ctx, userID) {
		return nil, false
	}
	return u.Abilities, true
}

// tokenStillActive returns a live token's abilities and owner; ok is false
// when it is gone, revoked or expired.
func (rt *Router) tokenStillActive(ctx context.Context, tokenID string) (abilities []string, owner string, ok bool) {
	now := time.Now()
	if rt.tokens != nil {
		rec, err := rt.tokens.GetAPITokenByID(ctx, tokenID)
		if err == nil && rec != nil {
			if rec.RevokedAt != nil || (rec.ExpiresAt != nil && !rec.ExpiresAt.After(now)) {
				return nil, "", false
			}
			return rec.Abilities, rec.OwnerUserID, true
		}
		if err != nil && !errors.Is(err, store.ErrAPITokenNotFound) {
			return nil, "", false
		}
	}
	if rt.authLib.tokens != nil {
		rec, err := rt.authLib.tokens.GetToken(ctx, tokenID)
		if err == nil {
			if rec.RevokedAt != nil || (rec.ExpiresAt != nil && !rec.ExpiresAt.After(now)) {
				return nil, "", false
			}
			return rec.Abilities, rec.OwnerLegacyID, true
		}
		if !errors.Is(err, authengine.ErrTokenUnknown) {
			return nil, "", false
		}
	}
	return nil, "", false
}

// voterAllowed re-resolves, at vote and at release time, that a principal is
// still live, holds AbilityDeploy and passes the team and environment checks
// for the approval's app. Any lookup failure counts as not allowed.
func (rt *Router) voterAllowed(ctx context.Context, a store.DeployApproval, typ, id string) bool {
	var abilities []string
	switch typ {
	case store.PrincipalTypeUser:
		ab, ok := rt.userStillActive(ctx, id)
		if !ok {
			return false
		}
		abilities = ab
	case store.PrincipalTypeToken:
		ab, owner, ok := rt.tokenStillActive(ctx, id)
		if !ok || owner == "" {
			return false
		}
		if _, ok := rt.userStillActive(ctx, owner); !ok {
			return false
		}
		abilities = ab
	default:
		return false
	}
	if !hasAbility(abilities, AbilityDeploy) {
		return false
	}
	scope, err := rt.principalTeamScope(ctx, typ, id)
	if err != nil {
		return false
	}
	if scope != nil {
		status, _, serr := rt.approvalScopeStatus(ctx, scope, a, AbilityDeploy, abilities)
		if serr != nil || status != 0 {
			return false
		}
	}
	if rt.policies == nil {
		return true
	}
	policies, err := rt.policies.ListPoliciesForPrincipal(ctx, typ, id)
	if err != nil {
		return false
	}
	allowed, hidden, err := rt.authorizeResourceInEnvironment(ctx, typ, id, abilities, policies, AbilityDeploy, "app:"+a.ServiceName)
	return err == nil && allowed && !hidden
}

// countApprovals counts distinct, still allowed approvers whose vote matches
// binding, never the requester. moved reports a vote cast on another digest.
func (rt *Router) countApprovals(ctx context.Context, a store.DeployApproval, votes []store.DeployApprovalVote, binding, digest string) (n int, moved bool) {
	seen := map[string]bool{}
	req := requesterKey(a)
	for _, v := range votes {
		if v.ApproverKey == req || seen[v.ApproverKey] {
			continue
		}
		if v.ImageDigest != digest {
			moved = true
			continue
		}
		if v.Binding != binding || !rt.voterAllowed(ctx, a, v.ApproverType, v.ApproverID) {
			continue
		}
		seen[v.ApproverKey] = true
	}
	return len(seen), moved
}

// approverGate is what gateRequiredApprovers decided for one approve call.
type approverGate struct {
	ready bool
	// multi is true when the environment needs more than one approver.
	multi bool
	// image, when set, is the digest pinned ref the release must deploy.
	image   string
	waiting deployApprovalDecisionResponse
}

// recordVote inserts the decider's vote, or replaces the same person's
// earlier vote when it no longer counts (token revoked, access lost, or a
// changed request), so a person is never locked out by a dead credential.
func (rt *Router) recordVote(ctx context.Context, vs approvalVoteStore, a store.DeployApproval, v store.DeployApprovalVote) (bool, error) {
	inserted, err := vs.RecordDeployApprovalVote(ctx, v)
	if err != nil || inserted {
		return inserted, err
	}
	votes, err := vs.ListDeployApprovalVotes(ctx, a.ID)
	if err != nil {
		return false, err
	}
	for _, prev := range votes {
		if prev.ApproverKey != v.ApproverKey {
			continue
		}
		if prev.Binding == v.Binding && prev.ImageDigest == v.ImageDigest && rt.voterAllowed(ctx, a, prev.ApproverType, prev.ApproverID) {
			return false, nil
		}
		return vs.ReplaceDeployApprovalVote(ctx, v, prev)
	}
	return false, nil
}

// gateRequiredApprovers records the decider's vote and reports whether the
// approval now has its N distinct approvers. N=1 returns ready at once so
// the single approver path is unchanged; the caller's claim still makes the
// release happen exactly once.
func (rt *Router) gateRequiredApprovers(ctx context.Context, a store.DeployApproval, deciderType, deciderID, deciderName string) (approverGate, error) {
	vs, ok := rt.approvalVotes()
	if !ok {
		return approverGate{ready: true}, nil
	}
	need, err := rt.requiredApprovers(ctx, vs, a.EnvironmentID)
	if err != nil {
		return approverGate{}, fmt.Errorf("api: approve deploy approval: read required approvers: %w", err)
	}
	if need <= 1 {
		return approverGate{ready: true}, nil
	}
	if deciderType != store.PrincipalTypeUser && deciderType != store.PrincipalTypeToken {
		return approverGate{}, errVoteNeedsPrincipal
	}
	human, err := rt.approvalHuman(ctx, deciderType, deciderID)
	if err != nil {
		return approverGate{}, fmt.Errorf("api: approve deploy approval: resolve approver: %w", err)
	}
	if human == "" {
		return approverGate{}, errVoteNeedsPrincipal
	}
	key := approverKey(human, deciderType, deciderID)
	if key == requesterKey(a) {
		return approverGate{}, errSameApprover
	}
	if !rt.voterAllowed(ctx, a, deciderType, deciderID) {
		return approverGate{}, errVoterNotAllowed
	}
	ref, digest, err := rt.pinnedImage(ctx, a)
	if err != nil {
		return approverGate{}, err
	}
	binding, err := rt.approvalBinding(ctx, a, digest)
	if err != nil {
		return approverGate{}, err
	}
	inserted, err := rt.recordVote(ctx, vs, a, store.DeployApprovalVote{
		ApprovalID: a.ID, ApproverKey: key, ApproverType: deciderType, ApproverID: deciderID,
		ApproverName: deciderName, Binding: binding, ImageDigest: digest, CreatedAt: store.FormatAuditTime(time.Now()),
	})
	if err != nil {
		return approverGate{}, fmt.Errorf("api: approve deploy approval: record vote: %w", err)
	}
	if inserted {
		rt.auditSystemEvent(ctx, deciderType+":"+deciderID, "deploy_approval.vote", a.ID+" approved by "+key+" ("+deciderName+")")
	}
	votes, err := vs.ListDeployApprovalVotes(ctx, a.ID)
	if err != nil {
		return approverGate{}, fmt.Errorf("api: approve deploy approval: list votes: %w", err)
	}
	got, moved := rt.countApprovals(ctx, a, votes, binding, digest)
	if moved {
		return approverGate{}, errTagMoved
	}
	if got < need {
		res := toDeployApprovalResource(a)
		res.ApprovalsReceived, res.ApprovalsRequired = got, need
		return approverGate{multi: true, waiting: deployApprovalDecisionResponse{Approval: res}}, nil
	}
	return approverGate{ready: true, multi: true, image: ref}, nil
}

// auditRelease records a multi approver release once its claim was won and
// the change ran, with the outcome.
func (rt *Router) auditRelease(ctx context.Context, a store.DeployApproval, deciderType, deciderID string, applyErr error) {
	outcome := "released"
	if applyErr != nil {
		outcome = "release failed, back to pending"
	}
	rt.auditSystemEvent(context.WithoutCancel(ctx), deciderType+":"+deciderID, "deploy_approval.release", a.ID+" "+outcome)
}

// withApproverCounts fills the progress fields for a pending approval in an
// environment that needs more than one approver.
func (rt *Router) withApproverCounts(ctx context.Context, a store.DeployApproval) deployApprovalResource {
	res := toDeployApprovalResource(a)
	vs, ok := rt.approvalVotes()
	if !ok || a.Status != store.DeployApprovalStatusPending {
		return res
	}
	need, err := rt.requiredApprovers(ctx, vs, a.EnvironmentID)
	if err != nil || need <= 1 {
		return res
	}
	res.ApprovalsRequired = need
	votes, err := vs.ListDeployApprovalVotes(ctx, a.ID)
	if err != nil || len(votes) == 0 {
		return res
	}
	// Counted against the digest the first vote saw, so a list never calls a registry.
	binding, err := rt.approvalBinding(ctx, a, votes[0].ImageDigest)
	if err != nil {
		return res
	}
	res.ApprovalsReceived, _ = rt.countApprovals(ctx, a, votes, binding, votes[0].ImageDigest)
	return res
}

// approverPolicyChange validates an environment PATCH's required_approvers
// and reports the stored value. A client message is returned on a bad value.
func (rt *Router) approverPolicyChange(ctx context.Context, envID string, n *int) (old int, msg string, err error) {
	setter, ok := rt.environments.(environmentApproverSetter)
	if !ok {
		if n != nil {
			return 0, "required approvers are not supported by this store", nil
		}
		return 1, "", nil
	}
	old, err = setter.EnvironmentRequiredApprovers(ctx, envID)
	if err != nil || n == nil {
		return old, "", err
	}
	if limit := rt.maxRequiredApprovers(ctx); *n < 1 || *n > limit {
		return old, fmt.Sprintf("required_approvers must be between 1 and %d", limit), nil
	}
	return old, "", nil
}

func (rt *Router) setRequiredApprovers(ctx context.Context, envID string, n int) error {
	setter, ok := rt.environments.(environmentApproverSetter)
	if !ok {
		return errors.New("api: required approvers store missing")
	}
	return setter.SetEnvironmentRequiredApprovers(ctx, envID, n, store.FormatAuditTime(time.Now()))
}
