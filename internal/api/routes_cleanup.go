package api

import "net/http"

func (rt *Router) registerCleanupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/cleanup", rt.requireAbility(AbilityRead, rt.handleGetCleanup))
	mux.HandleFunc("GET /api/v1/cleanup/runs", rt.requireAbility(AbilityRead, rt.handleListCleanupRuns))
	// A preview reads row counts only, but it is the same decision surface as a run, so it shares the Root gate.
	mux.HandleFunc("POST /api/v1/cleanup/{id}/preview", rt.requireAbility(AbilityRoot, rt.handlePreviewCleanup))
	// A run deletes data permanently.
	mux.HandleFunc("POST /api/v1/cleanup/{id}/run", rt.requireAbility(AbilityRoot, rt.handleRunCleanup))
	mux.HandleFunc("PUT /api/v1/cleanup/{id}/mode", rt.requireAbility(AbilityRoot, rt.handleSetCleanupMode))
}
