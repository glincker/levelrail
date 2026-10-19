package api

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/search"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	searchPrefixProject = "project:"
	// searchPrefixGlobal marks docs that name no tenant resource (templates, settings, docs).
	searchPrefixGlobal = "global:"
)

// searchAuthorizer decides search hits with the engine the path routes use:
// resourceScope for apps and databases, refAllowed for nodes, projects and
// environments, plain policy evaluation for everything else.
type searchAuthorizer struct {
	rt        *Router
	r         *http.Request
	abilities []string
	policies  []store.Policy
	scope     *resourceScope
	caller    *refCaller
}

func (rt *Router) newSearchCaller(r *http.Request) (search.Caller, error) {
	principalType, principalID, _, err := rt.callerPrincipal(r)
	if err != nil {
		return search.Caller{}, err
	}
	abilities, policies, scope, err := rt.callerScope(r)
	if err != nil {
		return search.Caller{}, err
	}
	authz := &searchAuthorizer{
		rt: rt, r: r, abilities: abilities, policies: policies, scope: scope,
		caller: &refCaller{principalType: principalType, principalID: principalID, abilities: abilities, policies: policies},
	}
	return search.Caller{PrincipalID: principalType + ":" + principalID, Authz: authz}, nil
}

// Allowed fails closed: an error resolving a reference hides the hit.
func (a *searchAuthorizer) Allowed(prefix, name, ability string) bool {
	switch prefix {
	case resourcePrefixApp, resourcePrefixDatabase:
		return a.scope.authorize(a.abilities, a.policies, ability, prefix, name)
	case resourcePrefixNode:
		return a.refAllowed(nodeRef(name), ability)
	case resourcePrefixEnvironment:
		return a.refAllowed(environmentRef(name), ability)
	case searchPrefixProject:
		return a.refAllowed(projectRef(name), ability)
	}
	return authorizeResource(a.abilities, a.policies, ability, prefix+name)
}

func (a *searchAuthorizer) refAllowed(ref resourceRef, ability string) bool {
	ok, err := a.rt.refAllowed(a.r.Context(), a.caller, ability, ref)
	if err != nil {
		a.rt.logger.Warn("api: search: authorize reference failed", slog.String("kind", string(ref.kind)), slog.String("id", ref.id), slog.String("error", fmt.Sprint(err)))
		return false
	}
	return ok
}
