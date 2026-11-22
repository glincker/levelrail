package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Actions only root may hold. They are not grantable, see store.CloudGrantActions.
const (
	cloudActionBuild  = "build"
	cloudActionDelete = "delete"
)

const cloudResourcePrefix = "cloud:"

// cloudResource names one scope under an account: the account alone, a
// compartment (or the provider's project level), or one instance. Compartment
// on an instance scope is its parent, so a compartment grant reaches it.
type cloudResource struct {
	AccountID   string
	Compartment string
	Instance    string
}

// String builds the IAM resource id, for example cloud:oracle-prod/instance:ocid1.x.
func (c cloudResource) String() string {
	switch {
	case c.Instance != "":
		return cloudResourcePrefix + c.AccountID + "/instance:" + c.Instance
	case c.Compartment != "":
		return cloudResourcePrefix + c.AccountID + "/compartment:" + c.Compartment
	default:
		return cloudResourcePrefix + c.AccountID
	}
}

// ancestors lists the IAM resource ids above this scope, nearest first.
func (c cloudResource) ancestors() []string {
	var out []string
	if c.Instance != "" && c.Compartment != "" {
		out = append(out, cloudResource{AccountID: c.AccountID, Compartment: c.Compartment}.String())
	}
	if c.Instance != "" || c.Compartment != "" {
		out = append(out, cloudResource{AccountID: c.AccountID}.String())
	}
	return out
}

// cloudAccountResource, cloudCompartmentResource and cloudInstanceResource are the scope builders routes use.
func cloudAccountResource(account string) cloudResource { return cloudResource{AccountID: account} }

func cloudCompartmentResource(account, compartment string) cloudResource {
	return cloudResource{AccountID: account, Compartment: compartment}
}

func cloudInstanceResource(account, compartment, instance string) cloudResource {
	return cloudResource{AccountID: account, Compartment: compartment, Instance: instance}
}

// cloudActionAbility maps a cloud action to the route ability the shared gate
// uses for audit and rate limits, and to the ability an IAM deny is matched on.
func cloudActionAbility(action string) string {
	switch action {
	case store.CloudActionView, store.CloudActionViewSpend:
		return AbilityRead
	case store.CloudActionOperate:
		return AbilityWrite
	default:
		return AbilityWriteSensitive
	}
}

// cloudAuthzInput is everything authorizeCloud needs, already loaded, so the
// decision itself is a pure function a table test can drive.
type cloudAuthzInput struct {
	Action   string
	Resource cloudResource
	Now      time.Time
	// Root is the caller's flat AbilityRoot.
	Root bool
	// Denied is true when an explicit IAM Deny matched the scope or an ancestor.
	Denied bool
	// Grants are the rows held by the caller or the caller's teams on this account.
	Grants []store.CloudGrant
	// OwnerTeamAdmin is true for an admin or owner of the account's own team.
	OwnerTeamAdmin bool
}

type cloudDecision struct {
	Allowed bool
	// Hidden asks the caller to answer 404: the caller has no standing on this scope at all.
	Hidden bool
}

// authorizeCloud decides in order: explicit deny, root, owner team admin, an
// unexpired grant on the scope or an ancestor, otherwise default deny.
func authorizeCloud(in cloudAuthzInput) cloudDecision {
	if in.Denied {
		return cloudDecision{}
	}
	if in.Root {
		return cloudDecision{Allowed: true}
	}
	if in.Action == cloudActionBuild || in.Action == cloudActionDelete {
		return cloudDecision{Hidden: !cloudHasStanding(in)}
	}
	if in.OwnerTeamAdmin {
		return cloudDecision{Allowed: true}
	}
	if slices.Contains(cloudEffectiveActions(in.Grants, in.Resource, in.Now), in.Action) {
		return cloudDecision{Allowed: true}
	}
	return cloudDecision{Hidden: !cloudHasStanding(in)}
}

func cloudHasStanding(in cloudAuthzInput) bool {
	return in.OwnerTeamAdmin || len(cloudEffectiveActions(in.Grants, in.Resource, in.Now)) > 0
}

// cloudEffectiveActions unions the actions of every unexpired grant that covers the scope.
func cloudEffectiveActions(grants []store.CloudGrant, res cloudResource, now time.Time) []string {
	var out []string
	for _, g := range grants {
		if g.AccountID != res.AccountID || !cloudGrantCovers(g, res) {
			continue
		}
		if g.ExpiresAt != nil && !now.Before(*g.ExpiresAt) {
			continue
		}
		for _, a := range g.Actions {
			if !slices.Contains(out, a) {
				out = append(out, a)
			}
		}
	}
	return out
}

func cloudGrantCovers(g store.CloudGrant, res cloudResource) bool {
	switch g.ResourceKind {
	case store.CloudResourceAccount:
		return true
	case store.CloudResourceCompartment:
		return res.Compartment != "" && g.ResourceRef == res.Compartment
	case store.CloudResourceInstance:
		return res.Instance != "" && g.ResourceRef == res.Instance
	}
	return false
}

// cloudGrantRequest is a proposed new grant checked against the grantor's own standing.
type cloudGrantRequest struct {
	Now      time.Time
	Root     bool
	Resource cloudResource
	// Actions are the ones the new grant would carry.
	Actions []string
	// AccountInstanceScoped is true when the account belongs to the instance, not a team.
	AccountInstanceScoped bool
	OwnerTeamAdmin        bool
	// GrantorGrants are the grantor's own rows on the account.
	GrantorGrants []store.CloudGrant
	// Expiry of the new grant, nil for none.
	ExpiresAt *time.Time
}

var errCloudNoEscalation = errors.New("a grant cannot hold actions the grantor lacks")

// authorizeCloudGrant enforces no escalation: the grantor needs share on the
// scope or above, may pass on only actions they hold there, and cannot outlive their own grant.
func authorizeCloudGrant(in cloudGrantRequest) error {
	if len(in.Actions) == 0 {
		return errors.New("a grant needs at least one action")
	}
	for _, a := range in.Actions {
		if !slices.Contains(store.CloudGrantActions(), a) {
			return errors.New("action " + a + " cannot be granted")
		}
	}
	if in.Root {
		return nil
	}
	if in.OwnerTeamAdmin && !in.AccountInstanceScoped {
		return nil
	}
	// Only root hands out access to an instance-owned account at account level.
	if in.AccountInstanceScoped && in.Resource.Compartment == "" && in.Resource.Instance == "" {
		return errors.New("only root can grant access to an instance-owned account")
	}
	held := cloudEffectiveActions(in.GrantorGrants, in.Resource, in.Now)
	if !slices.Contains(held, store.CloudActionShare) {
		return errors.New("sharing requires the share action on this scope")
	}
	for _, a := range in.Actions {
		if !slices.Contains(held, a) {
			return errCloudNoEscalation
		}
	}
	if latest, bounded := cloudLatestHeldExpiry(in.GrantorGrants, in.Resource, in.Now); bounded && (in.ExpiresAt == nil || in.ExpiresAt.After(latest)) {
		return errors.New("a grant cannot outlive the grantor's own access")
	}
	return nil
}

// cloudLatestHeldExpiry returns the latest expiry among live grants covering the scope.
// bounded is false when none covers it or any of them never expires.
func cloudLatestHeldExpiry(grants []store.CloudGrant, res cloudResource, now time.Time) (latest time.Time, bounded bool) {
	for _, g := range grants {
		if g.AccountID != res.AccountID || !cloudGrantCovers(g, res) {
			continue
		}
		if g.ExpiresAt == nil {
			return time.Time{}, false
		}
		if !now.Before(*g.ExpiresAt) {
			continue
		}
		bounded = true
		if g.ExpiresAt.After(latest) {
			latest = *g.ExpiresAt
		}
	}
	return latest, bounded
}

// cloudAuthzStore is the store surface the wrapper needs; *store.DB satisfies it.
type cloudAuthzStore interface {
	GetCloudAccount(ctx context.Context, id string) (store.CloudAccount, error)
	ListCloudGrantsForPrincipals(ctx context.Context, accountID string, principals []store.CloudPrincipal) ([]store.CloudGrant, error)
	UserTeamRoles(ctx context.Context, userID string) (map[string]string, error)
}

// cloudCaller is the resolved standing of one request principal.
type cloudCaller struct {
	principals []store.CloudPrincipal
	teamRoles  map[string]string
}

// cloudCallerFor maps a session user or a token to its grant principals.
// A token acts as its owner and an ownerless token has none, so it only passes as root.
func (rt *Router) cloudCallerFor(ctx context.Context, db cloudAuthzStore, principalType, principalID string) (cloudCaller, error) {
	userID := principalID
	if principalType == store.PrincipalTypeToken {
		rec, err := rt.tokens.GetAPITokenByID(ctx, principalID)
		if err != nil {
			return cloudCaller{}, err
		}
		userID = rec.OwnerUserID
	}
	if userID == "" {
		return cloudCaller{}, nil
	}
	roles, err := db.UserTeamRoles(ctx, userID)
	if err != nil {
		return cloudCaller{}, err
	}
	if principalType == store.PrincipalTypeToken {
		if roles, err = rt.scopeTokenRoles(ctx, principalID, roles); err != nil {
			return cloudCaller{}, err
		}
	}
	c := cloudCaller{teamRoles: roles, principals: []store.CloudPrincipal{{Type: store.CloudPrincipalUser, ID: userID}}}
	for team := range roles {
		c.principals = append(c.principals, store.CloudPrincipal{Type: store.CloudPrincipalTeam, ID: team})
	}
	return c, nil
}

// scopeTokenRoles narrows a token owner's team roles to the token's own scope.
func (rt *Router) scopeTokenRoles(ctx context.Context, tokenID string, roles map[string]string) (map[string]string, error) {
	scope, err := rt.principalTeamScope(ctx, store.PrincipalTypeToken, tokenID)
	if err != nil {
		return nil, err
	}
	if scope == nil {
		return roles, nil
	}
	out := map[string]string{}
	for team := range roles {
		if role := scope.roleOfTeam(team); role != "" {
			out[team] = role
		}
	}
	return out, nil
}

// tokenScopeAllowsAccount is false when a scoped token cannot reach the account's team.
func (rt *Router) tokenScopeAllowsAccount(ctx context.Context, principalType, principalID string, acct store.CloudAccount) (bool, error) {
	if principalType != store.PrincipalTypeToken {
		return true, nil
	}
	scope, err := rt.principalTeamScope(ctx, principalType, principalID)
	if err != nil || scope == nil {
		return err == nil, err
	}
	if acct.TeamID == "" {
		return scope.instance, nil
	}
	return scope.allowsTeam(acct.TeamID), nil
}

// requireCloudAccess gates a cloud route on one action over the scope resourceFn
// names. It rides requireAbilityDecided, so audit, rate limits, revoked tokens
// and the team gate are inherited. A caller with no standing gets 404.
func (rt *Router) requireCloudAccess(db cloudAuthzStore, action string, resourceFn func(*http.Request) cloudResource, next http.HandlerFunc) http.HandlerFunc {
	ability := cloudActionAbility(action)
	return func(w http.ResponseWriter, r *http.Request) {
		res := resourceFn(r)
		var hidden bool
		decide := func(ctx context.Context, principalType, principalID string, abilities []string) (bool, error) {
			in := cloudAuthzInput{Action: action, Resource: res, Now: time.Now().UTC(), Root: hasAbility(abilities, AbilityRoot)}
			if !in.Root {
				allowed, notVisible, err := rt.decideCloudForCaller(ctx, db, principalType, principalID, abilities, ability, &in)
				hidden = notVisible
				return allowed, err
			}
			if scopedOut, err := rt.cloudScopedOut(ctx, db, principalType, principalID, res); err != nil || scopedOut {
				hidden = scopedOut
				return false, err
			}
			denied, err := rt.cloudDenied(ctx, principalType, principalID, ability, res)
			if err != nil {
				return false, err
			}
			in.Denied = denied
			return authorizeCloud(in).Allowed, nil
		}
		hideUnseen := func(w http.ResponseWriter, r *http.Request) {
			if hidden {
				writeError(w, http.StatusNotFound, "cloud account not found")
				return
			}
			next(w, r)
		}
		rt.requireAbilityDecided(ability, decide, hideUnseen)(w, r)
	}
}

func (rt *Router) decideCloudForCaller(ctx context.Context, db cloudAuthzStore, principalType, principalID string, abilities []string, ability string, in *cloudAuthzInput) (allowed, hidden bool, err error) {
	// A token never exceeds its own abilities, whatever its owner was granted.
	if principalType == store.PrincipalTypeToken && !hasAbility(abilities, ability) {
		return false, false, nil
	}
	caller, err := rt.cloudCallerFor(ctx, db, principalType, principalID)
	if err != nil {
		return false, false, err
	}
	acct, err := db.GetCloudAccount(ctx, in.Resource.AccountID)
	if errors.Is(err, store.ErrCloudAccountNotFound) {
		return false, true, nil
	}
	if err != nil {
		return false, false, err
	}
	// Compartment grants only apply once the instance's compartment is known; unknown stays unscoped.
	if in.Resource.Instance != "" && in.Resource.Compartment == "" && acct.Provider == "oracle" {
		in.Resource.Compartment = rt.instanceCompartment(ctx, acct, in.Resource.Instance)
	}
	if in.Denied, err = rt.cloudDenied(ctx, principalType, principalID, ability, in.Resource); err != nil {
		return false, false, err
	}
	if ok, err := rt.tokenScopeAllowsAccount(ctx, principalType, principalID, acct); err != nil || !ok {
		return false, !ok, err
	}
	in.OwnerTeamAdmin = acct.TeamID != "" && isTeamAdminRole(caller.teamRoles[acct.TeamID])
	if in.Grants, err = db.ListCloudGrantsForPrincipals(ctx, in.Resource.AccountID, caller.principals); err != nil {
		return false, false, err
	}
	d := authorizeCloud(*in)
	return d.Allowed, d.Hidden, nil
}

// cloudScopedOut reports a team scoped token reaching for an account outside its scope.
func (rt *Router) cloudScopedOut(ctx context.Context, db cloudAuthzStore, principalType, principalID string, res cloudResource) (bool, error) {
	if principalType != store.PrincipalTypeToken || res.AccountID == "" {
		return false, nil
	}
	acct, err := db.GetCloudAccount(ctx, res.AccountID)
	if errors.Is(err, store.ErrCloudAccountNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	ok, err := rt.tokenScopeAllowsAccount(ctx, principalType, principalID, acct)
	return !ok, err
}

func (rt *Router) cloudDenied(ctx context.Context, principalType, principalID, ability string, res cloudResource) (bool, error) {
	policies, err := rt.policies.ListPoliciesForPrincipal(ctx, principalType, principalID)
	if err != nil {
		return false, err
	}
	matched, allowed := evaluatePolicies(policies, ability, res.String(), res.ancestors()...)
	return matched && !allowed, nil
}

func isTeamAdminRole(role string) bool {
	return role == store.TeamRoleOwner || role == store.TeamRoleAdmin
}

// instanceCompartment asks the provider which compartment holds an instance; "" when it cannot tell.
func (rt *Router) instanceCompartment(ctx context.Context, a store.CloudAccount, instanceID string) string {
	p, ok, _, _ := rt.resolveNodeProvisionerForAccount(ctx, a.Provider, a.ID)
	if !ok {
		return ""
	}
	im, ok := p.(provision.InstanceManager)
	if !ok {
		return ""
	}
	d, err := im.DescribeInstance(ctx, instanceID)
	if err != nil {
		return ""
	}
	return d.CompartmentID
}

// dropCloudGrantsFor removes the cloud grants a deleted user or team held.
func (rt *Router) dropCloudGrantsFor(ctx context.Context, principalType, principalID string) {
	st, ok := rt.apps.(interface {
		DeleteCloudGrantsForPrincipal(ctx context.Context, principalType, principalID string) error
	})
	if !ok {
		return
	}
	if err := st.DeleteCloudGrantsForPrincipal(ctx, principalType, principalID); err != nil {
		rt.logger.Warn("api: drop cloud grants failed", slog.String("principal_type", principalType))
	}
}
