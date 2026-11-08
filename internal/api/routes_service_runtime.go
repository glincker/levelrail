package api

import "net/http"

// registerServiceRuntimeRoutes registers the per-service runtime options read and write.
func (rt *Router) registerServiceRuntimeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/services/{service}/runtime", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetServiceRuntime))
	mux.HandleFunc("PUT /api/v1/apps/{name}/services/{service}/runtime", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handlePutServiceRuntime))
}
