package api

import "net/http"

// registerAutoHostRoutes registers the per-app automatic hostname opt-out and the certificate budget meter.
func (rt *Router) registerAutoHostRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/auto-host", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppAutoHost))
	mux.HandleFunc("PUT /api/v1/apps/{name}/auto-host", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetAppAutoHost))
	mux.HandleFunc("GET /api/v1/certificates/budget", rt.requireAbility(AbilityRead, rt.handleGetCertBudget))
}
