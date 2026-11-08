package api

import "net/http"

// registerAutoHostRoutes registers the per-app automatic hostname opt-out and the certificate budget meter.
func (rt *Router) registerAutoHostRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/protection", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppProtection))
	mux.HandleFunc("PUT /api/v1/apps/{name}/protection", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handlePutAppProtection))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/protection", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleDeleteAppProtection))
	mux.HandleFunc("GET /api/v1/apps/{name}/auto-host", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppAutoHost))
	mux.HandleFunc("PUT /api/v1/apps/{name}/auto-host", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetAppAutoHost))
	mux.HandleFunc("GET /api/v1/certificates/budget", rt.requireAbility(AbilityRead, rt.handleGetCertBudget))
	mux.HandleFunc("GET /api/v1/doctor/reachability", rt.requireAbility(AbilityRead, rt.handleDoctorReachability))
}
