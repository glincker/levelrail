package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/glincker/levelrail/internal/dbauthz"
	"github.com/glincker/levelrail/internal/store"
)

// dbPolicyEngine adapts the router's IAM evaluation to dbauthz.PolicyEngine.
// It loads the caller's policies once per request.
type dbPolicyEngine struct {
	rt       *Router
	policies []store.Policy
	loaded   bool
}

func (e *dbPolicyEngine) Evaluate(ctx context.Context, c dbauthz.Caller, ability string, action dbauthz.Action, resource string) (dbauthz.Verdict, error) {
	if !e.loaded {
		p, err := e.rt.policies.ListPoliciesForPrincipal(ctx, c.PrincipalType, c.PrincipalID)
		if err != nil {
			return dbauthz.Verdict{}, err
		}
		e.policies, e.loaded = p, true
	}
	return e.rt.dbEvaluate(ctx, c.PrincipalType, c.PrincipalID, c.Abilities, e.policies, ability, action, resource)
}

// dbEvaluate decides one db action. Order: visibility (hidden gives 404), then
// any explicit Deny on the ability or the action, then the effective ability
// path unchanged, then a fallback that accepts only a policy Allow naming the
// db action, never the base ability.
func (rt *Router) dbEvaluate(ctx context.Context, pt, pid string, abilities []string, policies []store.Policy, ability string, action dbauthz.Action, resource string) (dbauthz.Verdict, error) {
	extra, err := rt.dbEnvironmentResources(ctx, policies, resource)
	if err != nil {
		return dbauthz.Verdict{}, err
	}
	return rt.dbEvaluateWith(ctx, pt, pid, abilities, policies, ability, action, resource, extra)
}

// dbEnvironmentResources resolves the environment resources of one database
// once, so callers deciding several actions reuse it.
func (rt *Router) dbEnvironmentResources(ctx context.Context, policies []store.Policy, resource string) ([]string, error) {
	prefix, name, ok := splitResource(resource)
	if !ok || !policiesMentionEnvironments(policies) {
		return nil, nil
	}
	ref, err := rt.environmentForResource(ctx, prefix, name)
	if err != nil {
		return nil, fmt.Errorf("api: resource environment: %w", err)
	}
	return environmentResources(ref), nil
}

func (rt *Router) dbEvaluateWith(ctx context.Context, pt, pid string, abilities []string, policies []store.Policy, ability string, action dbauthz.Action, resource string, extra []string) (dbauthz.Verdict, error) {
	allowed, hidden, err := rt.authorizeResourceInEnvironment(ctx, pt, pid, abilities, policies, ability, resource)
	if err != nil {
		return dbauthz.Verdict{}, err
	}
	if hidden {
		return dbauthz.Verdict{Hidden: true}, nil
	}
	for _, name := range append([]string{ability}, dbauthz.DenyNames(action)...) {
		if matched, ok := evaluatePolicies(policies, name, resource, extra...); matched && !ok {
			return dbauthz.Verdict{Denied: true}, nil
		}
	}
	if allowed {
		return dbauthz.Verdict{Allowed: true}, nil
	}
	// Policy Allow only: a nil base ability set means authorizeResource can
	// grant nothing but a matching Allow statement for the db action.
	byAction, byHidden, err := rt.authorizeResourceInEnvironment(ctx, pt, pid, nil, policies, string(action), resource)
	if err != nil {
		return dbauthz.Verdict{}, err
	}
	return dbauthz.Verdict{Allowed: byAction && !byHidden, ByAction: byAction && !byHidden}, nil
}

// requireAbilityForDatabase gates one explorer route through dbauthz.Resolve.
// ability must equal the action's effective ability, so the route table and
// generated reference keep showing it; a mismatch is a wiring bug and panics at startup.
// Query-history delete and saved-query create/delete stay on db:read on purpose:
// the old gate was read:sensitive and behavior is unchanged.
func (rt *Router) requireAbilityForDatabase(ability string, action dbauthz.Action, next http.HandlerFunc) http.HandlerFunc {
	if dbauthz.EffectiveAbility(action) != ability {
		panic(fmt.Sprintf("api: database route ability %q does not match action %q", ability, action))
	}
	return func(w http.ResponseWriter, r *http.Request) {
		resolver := &dbauthz.Resolver{Engine: &dbPolicyEngine{rt: rt}}
		resource := databaseResourceFromPath(r)
		var hidden bool
		decision := func(ctx context.Context, principalType, principalID string, abilities []string) (bool, error) {
			d, err := resolver.Resolve(ctx, dbauthz.Caller{PrincipalType: principalType, PrincipalID: principalID, Abilities: abilities}, resource, action)
			hidden = d.Hidden
			if err != nil {
				return false, err
			}
			return d.Allowed || d.Hidden, nil
		}
		gated := func(w http.ResponseWriter, r *http.Request) {
			if hidden {
				noteDenied(r)
				writeError(w, http.StatusNotFound, hiddenResourceMessage(resource))
				return
			}
			next(w, r)
		}
		rt.withDatabaseAudit(ability, action, rt.requireAbilityDecided(ability, decision, gated))(w, r)
	}
}

// impliedActions lists the action and the stronger ones that also grant it on one database:
// admin covers write, and write covers read, so a branch-only admin grant can read and wait.
func impliedActions(a dbauthz.Action) []dbauthz.Action {
	switch a {
	case dbauthz.ActionRead:
		return []dbauthz.Action{dbauthz.ActionRead, dbauthz.ActionWrite, dbauthz.ActionAdmin}
	case dbauthz.ActionWrite:
		return []dbauthz.Action{dbauthz.ActionWrite, dbauthz.ActionAdmin}
	}
	return []dbauthz.Action{a}
}

// weakerActions lists the actions below a that an explicit Deny can still block.
func weakerActions(a dbauthz.Action) []dbauthz.Action {
	switch a {
	case dbauthz.ActionAdmin:
		return []dbauthz.Action{dbauthz.ActionRead, dbauthz.ActionWrite}
	case dbauthz.ActionWrite:
		return []dbauthz.Action{dbauthz.ActionRead}
	}
	return nil
}

// resolveImplied answers one action on one database. The requested action is resolved first and an
// explicit Deny on it ends the question. A stronger grant is tried only when the first answer is a
// plain lack of ability, and it never overrides a Deny on any weaker action.
func resolveImplied(ctx context.Context, resolver *dbauthz.Resolver, caller dbauthz.Caller, resource string, action dbauthz.Action) (allowed, hidden bool, err error) {
	first, err := resolver.Resolve(ctx, caller, resource, action)
	if err != nil {
		return false, false, err
	}
	switch {
	case first.Hidden:
		return false, true, nil
	case first.Allowed:
		return true, false, nil
	case first.PolicyDecision == dbauthz.PolicyDenied:
		return false, false, nil
	}
	for _, stronger := range impliedActions(action)[1:] {
		d, err := resolver.Resolve(ctx, caller, resource, stronger)
		if err != nil {
			return false, false, err
		}
		if d.Hidden {
			return false, true, nil
		}
		if !d.Allowed {
			continue
		}
		for _, weaker := range weakerActions(stronger) {
			w, err := resolver.Resolve(ctx, caller, resource, weaker)
			if err != nil {
				return false, false, err
			}
			if w.Hidden {
				return false, true, nil
			}
			if w.PolicyDecision == dbauthz.PolicyDenied {
				return false, false, nil
			}
		}
		return true, false, nil
	}
	return false, false, nil
}

// requireAbilityForBranch gates a branch route on the branch itself, not on the parent, so a token
// limited to branch databases can manage them without any right on the parent beyond creating them.
func (rt *Router) requireAbilityForBranch(ability string, action dbauthz.Action, next http.HandlerFunc) http.HandlerFunc {
	if dbauthz.EffectiveAbility(action) != ability {
		panic(fmt.Sprintf("api: database route ability %q does not match action %q", ability, action))
	}
	return func(w http.ResponseWriter, r *http.Request) {
		resolver := &dbauthz.Resolver{Engine: &dbPolicyEngine{rt: rt}}
		resource := dbauthz.ResourcePrefix + r.PathValue("branch")
		parent := dbauthz.ResourcePrefix + r.PathValue("name")
		// Changing a branch needs write on the database in the path, looking needs read; this runs
		// before the branch is looked up or the body is read.
		parentAction := dbauthz.ActionWrite
		if action == dbauthz.ActionRead {
			parentAction = dbauthz.ActionRead
		}
		var hidden bool
		decision := func(ctx context.Context, principalType, principalID string, abilities []string) (bool, error) {
			caller := dbauthz.Caller{PrincipalType: principalType, PrincipalID: principalID, Abilities: abilities}
			pAllowed, pHidden, err := resolveImplied(ctx, resolver, caller, parent, parentAction)
			if err != nil || pHidden {
				hidden = pHidden
				return pHidden, err
			}
			if !pAllowed {
				return false, nil
			}
			if d, err := resolver.Resolve(ctx, caller, parent, dbauthz.ActionRead); err != nil || d.PolicyDecision == dbauthz.PolicyDenied {
				return false, err
			}
			allowed, h, err := resolveImplied(ctx, resolver, caller, resource, action)
			hidden = h
			return allowed || h, err
		}
		gated := func(w http.ResponseWriter, r *http.Request) {
			if hidden {
				noteDenied(r)
				writeError(w, http.StatusNotFound, "branch not found")
				return
			}
			next(w, r)
		}
		rt.withDatabaseAudit(ability, action, rt.requireAbilityDecided(ability, decision, gated))(w, r)
	}
}
