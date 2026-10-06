package api

import "net/http"

func (rt *Router) registerIAMTemplateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/iam/policy-templates", rt.requireAbility(AbilityRead, rt.handleListPolicyTemplates))
	mux.HandleFunc("POST /api/v1/iam/policy-templates/{id}/apply", rt.requireAbility(AbilityRoot, rt.handleApplyPolicyTemplate))
}
