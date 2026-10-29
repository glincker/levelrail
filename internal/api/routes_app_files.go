package api

import "net/http"

// Inline files carry config that may hold secrets, so writes are sensitive.
func (rt *Router) registerAppFilesRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/files", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListAppFiles))
	mux.HandleFunc("PUT /api/v1/apps/{name}/files", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetAppFile))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/files", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleDeleteAppFile))
}
