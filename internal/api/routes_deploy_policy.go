package api

import "net/http"

// registerDeployPolicyRoutes wires deploy guardrail policies. Changing a
// rule set is root only because it governs every deploy in its scope.
func (rt *Router) registerDeployPolicyRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/deploy-policies", rt.requireAbility(AbilityRead, rt.handleListDeployPolicies))
	mux.HandleFunc("POST /api/v1/deploy-policies/check", rt.requireAbility(AbilityRead, rt.handleCheckDeployPolicy))
	mux.HandleFunc("PUT /api/v1/deploy-policies/{scope}/{id}", rt.requireAbility(AbilityRoot, rt.handleSetDeployPolicy))
	mux.HandleFunc("DELETE /api/v1/deploy-policies/{scope}/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteDeployPolicy))
}
