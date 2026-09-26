package api

import "net/http"

func (rt *Router) registerPreviewRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/preview", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetPreview))
	mux.HandleFunc("PUT /api/v1/apps/{name}/preview", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handlePutPreview))
	mux.HandleFunc("POST /api/v1/apps/{name}/preview/capture", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleCapturePreview))
	mux.HandleFunc("POST /api/v1/apps/{name}/preview/prune", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handlePrunePreview))
	mux.HandleFunc("GET /api/v1/apps/{name}/preview/history", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListPreviews))
	mux.HandleFunc("GET /api/v1/apps/{name}/deployments/{id}/preview", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetPreviewImage))
}
