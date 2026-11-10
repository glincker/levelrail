package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

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

	// One listing per request answers every hit's stack-sharing check.
	groupsOnce sync.Once
	services   map[string]bool
	groups     map[string][]string
	groupsErr  error
}

func (a *searchAuthorizer) loadGroups() {
	svcs, err := a.rt.apps.ListDesiredServices(a.r.Context())
	if err != nil {
		a.groupsErr = err
		return
	}
	a.services, a.groups = make(map[string]bool, len(svcs)), map[string][]string{}
	for _, s := range svcs {
		a.services[s.Name] = true
		if s.AppID != "" && s.AppID != s.Name {
			a.groups[s.AppID] = append(a.groups[s.AppID], s.Name)
		}
	}
}

// appAllowed is the REST rule (canReadDeploymentApp over callerAppVisibility):
// the name and every service sharing it must pass the compose aware check.
func (a *searchAuthorizer) appAllowed(ability, name string) bool {
	a.groupsOnce.Do(a.loadGroups)
	if a.groupsErr != nil {
		return false
	}
	return stackReadable(name, a.groups[name], func(n string) bool { return a.composeAwareAllowed(ability, n) })
}

// composeAwareAllowed mirrors composeAwareRead from the request's one listing,
// so a team owned stack is judged by its members and no hit costs a query.
func (a *searchAuthorizer) composeAwareAllowed(ability, app string) bool {
	base := a.scope.authorize(a.abilities, a.policies, ability, resourcePrefixApp, app)
	if a.scope.teams == nil || a.rt.teamDB() == nil {
		return base
	}
	owned := a.scope.owner(resourcePrefixApp, app) != nil
	members := a.groups[app]
	if !owned && (a.services[app] || len(members) == 0) {
		return base
	}
	if owned && !base {
		return false
	}
	for _, m := range members {
		if !a.scope.authorizeAsMember(a.abilities, a.policies, ability, resourcePrefixApp, app, m) {
			return false
		}
	}
	return true
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
	case resourcePrefixApp:
		return a.appAllowed(ability, name)
	case resourcePrefixDatabase:
		return a.scope.authorize(a.abilities, a.policies, ability, prefix, name)
	case resourcePrefixNode:
		return a.refAllowed(nodeRef(name), ability)
	case resourcePrefixEnvironment:
		return a.refAllowed(environmentRef(name), ability)
	case searchPrefixProject:
		return a.refAllowed(projectRef(name), ability)
	}
	if prefix == searchPrefixGlobal && a.tenant() && !tenantGlobalHit(name) {
		return false
	}
	return authorizeResource(a.abilities, a.policies, ability, prefix+name)
}

func (a *searchAuthorizer) tenant() bool {
	return a.scope != nil && a.scope.teams != nil && !a.scope.teams.instance
}

// tenantGlobalHit: tags and settings are instance wide, so a tenant only sees catalog and help hits.
func tenantGlobalHit(name string) bool {
	return strings.HasPrefix(name, "template:") || strings.HasPrefix(name, "doc:")
}

func (a *searchAuthorizer) refAllowed(ref resourceRef, ability string) bool {
	ok, err := a.rt.refAllowed(a.r.Context(), a.caller, ability, ref)
	if err != nil {
		a.rt.logger.Warn("api: search: authorize reference failed", slog.String("kind", string(ref.kind)), slog.String("id", ref.id), slog.String("error", fmt.Sprint(err)))
		return false
	}
	return ok
}
