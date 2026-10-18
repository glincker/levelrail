package api

import "net/http"

func (rt *Router) registerBuildPerformanceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/builds", rt.requireAbility(AbilityRead, rt.handleListBuilds))
	mux.HandleFunc("GET /api/v1/builds/cache", rt.requireAbility(AbilityRead, rt.handleBuildCache))
	// Pruning frees disk shared by every app's builds, so write-sensitive even as a preview.
	mux.HandleFunc("POST /api/v1/builds/cache/prune", rt.requireAbility(AbilityWriteSensitive, rt.handleBuildCachePrune))
	mux.HandleFunc("GET /api/v1/builds/{id}", rt.requireAbility(AbilityRead, rt.handleGetBuild))
	mux.HandleFunc("POST /api/v1/apps/{name}/builds/{id}/cancel", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleCancelBuild))
	mux.HandleFunc("POST /api/v1/apps/{name}/builds/{id}/retry", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleRetryBuild))
	mux.HandleFunc("GET /api/v1/apps/{name}/build-stats", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleBuildStats))
	mux.HandleFunc("GET /api/v1/apps/{name}/build-speed", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleBuildSpeed))
	mux.HandleFunc("GET /api/v1/build-limits", rt.requireAbility(AbilityRead, rt.handleGetBuildLimits))
	// Limits decide when every app's builds are killed, so root only.
	mux.HandleFunc("PUT /api/v1/build-limits", rt.requireAbility(AbilityRoot, rt.handleSetBuildLimits))
	mux.HandleFunc("DELETE /api/v1/build-limits", rt.requireAbility(AbilityRoot, rt.handleResetBuildLimits))
}
