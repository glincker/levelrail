package api

import "net/http"

// registerAppTypeRoutes wires app types (ADR 052). Writing a type is root
// only because a published type constrains every app that pins it.
func (rt *Router) registerAppTypeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/app-types", rt.requireAbility(AbilityRead, rt.handleListAppTypes))
	mux.HandleFunc("POST /api/v1/app-types", rt.requireAbility(AbilityRoot, rt.handleSaveAppType))
	mux.HandleFunc("POST /api/v1/app-types/{name}/publish", rt.requireAbility(AbilityRoot, rt.handlePublishAppType))
	mux.HandleFunc("POST /api/v1/app-types/{name}/check", rt.requireAbility(AbilityRead, rt.handleCheckAppType))
}
