package api

import (
	"net/http"
	"slices"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// authorizeDatabaseRefs checks the caller may see each named database, using
// the same engine as a /databases/{name} route (IAM Deny, environment
// visibility, team ownership). A refusal answers like an unknown database so
// the response is not an existence oracle. It writes the response itself.
func (rt *Router) authorizeDatabaseRefs(w http.ResponseWriter, r *http.Request, names ...string) bool {
	if len(names) == 0 {
		return true
	}
	principalType, principalID, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		rt.internalError(w, "api: database ref authz: resolve caller", err)
		return false
	}
	policies, err := rt.policies.ListPoliciesForPrincipal(r.Context(), principalType, principalID)
	if err != nil {
		rt.internalError(w, "api: database ref authz: list policies", err)
		return false
	}
	for _, name := range slices.Compact(slices.Sorted(slices.Values(names))) {
		allowed, hidden, err := rt.authorizeResourceInEnvironment(r.Context(), principalType, principalID, abilities, policies, AbilityRead, resourcePrefixDatabase+name)
		if err != nil {
			rt.internalError(w, "api: database ref authz: decide", err)
			return false
		}
		if !allowed || hidden {
			writeError(w, http.StatusBadRequest, "unknown database")
			return false
		}
	}
	return true
}

// authorizeSpecDatabaseRefs applies authorizeDatabaseRefs to every database a
// spec's { from: ... } env vars reference.
func (rt *Router) authorizeSpecDatabaseRefs(w http.ResponseWriter, r *http.Request, services map[string]spec.Service) bool {
	return rt.authorizeDatabaseRefs(w, r, deploy.DatabaseRefs(services)...)
}
