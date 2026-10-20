package api

import "net/http"

// registerIngressEdgeRoutes: reading is AbilityRead; changing instance edge
// limits or the trusted proxy list is AbilityRoot like the other ingress
// settings, and a per-app body cap is AbilityDeploy like other app settings.
func (rt *Router) registerIngressEdgeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/settings/ingress/edge", rt.requireAbility(AbilityRead, rt.handleGetIngressEdge))
	mux.HandleFunc("PUT /api/v1/settings/ingress/edge", rt.requireAbility(AbilityRoot, rt.handlePutIngressEdge))
	mux.HandleFunc("POST /api/v1/settings/ingress/edge/trusted-proxies/apply-detected", rt.requireAbility(AbilityRoot, rt.handleApplyDetectedProxy))
	mux.HandleFunc("GET /api/v1/apps/{name}/ingress-limits", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppIngressLimits))
	mux.HandleFunc("PUT /api/v1/apps/{name}/ingress-limits", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handlePutAppIngressLimits))
}
