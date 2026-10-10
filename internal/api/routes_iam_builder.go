package api

import "net/http"

func (rt *Router) registerIAMBuilderRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/iam/catalog", rt.requireAbility(AbilityRead, rt.handleIAMCatalog))
	mux.HandleFunc("GET /api/v1/iam/resources", rt.requireAbility(AbilityRead, rt.handleIAMResources))
	mux.HandleFunc("POST /api/v1/iam/resources/match", rt.requireAbility(AbilityRead, rt.handleIAMMatch))
	mux.HandleFunc("GET /api/v1/iam/principals", rt.requireAbility(AbilityRead, rt.handleIAMPrincipals))
	mux.HandleFunc("GET /api/v1/iam/principals/{principal_type}/{principal_id}/effective", rt.requireAbility(AbilityRead, rt.handleIAMEffective))
	mux.HandleFunc("GET /api/v1/iam/simulate", rt.requireAbility(AbilityRead, rt.handleIAMSimulate))
	mux.HandleFunc("GET /api/v1/iam/analyze", rt.requireAbility(AbilityRead, rt.handleIAMAnalyze))
	mux.HandleFunc("POST /api/v1/iam/policies/validate", rt.requireAbility(AbilityRead, rt.handleValidatePolicy))
	mux.HandleFunc("POST /api/v1/iam/preview", rt.requireAbility(AbilityRead, rt.handleIAMPreview))
	mux.HandleFunc("GET /api/v1/iam/policies/{id}/versions", rt.requireAbility(AbilityRead, rt.handleListPolicyVersions))
	mux.HandleFunc("POST /api/v1/iam/policy-templates/{id}/render", rt.requireAbility(AbilityRead, rt.handleRenderPolicyTemplate))
}
