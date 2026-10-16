package api

import "net/http"

func (rt *Router) registerBuildPerformanceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/build-stats", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleBuildStats))
	mux.HandleFunc("GET /api/v1/build-limits", rt.requireAbility(AbilityRead, rt.handleGetBuildLimits))
	// Limits decide when every app's builds are killed, so root only.
	mux.HandleFunc("PUT /api/v1/build-limits", rt.requireAbility(AbilityRoot, rt.handleSetBuildLimits))
	mux.HandleFunc("DELETE /api/v1/build-limits", rt.requireAbility(AbilityRoot, rt.handleResetBuildLimits))
}
