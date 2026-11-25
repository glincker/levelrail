package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/dbauthz"
	"github.com/GLINCKER/levelrail/internal/store"
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
				writeError(w, http.StatusNotFound, hiddenResourceMessage(resource))
				return
			}
			next(w, r)
		}
		rt.requireAbilityDecided(ability, decision, gated)(w, r)
	}
}
