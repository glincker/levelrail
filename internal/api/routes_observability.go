package api

import "net/http"

// registerObservabilityRoutes wires the provider presets and the exports
// made from them. Changing an export needs sensitive write, the catalog is a
// plain read.
func (rt *Router) registerObservabilityRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/observability/presets", rt.requireAbility(AbilityRead, rt.handleListObservabilityPresets))
	mux.HandleFunc("GET /api/v1/observability/exports", rt.requireAbility(AbilityReadSensitive, rt.handleListObservabilityExports))
	mux.HandleFunc("POST /api/v1/observability/exports", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateObservabilityExport))
	mux.HandleFunc("DELETE /api/v1/observability/exports/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteObservabilityExport))
	mux.HandleFunc("POST /api/v1/observability/exports/{id}/test", rt.requireAbility(AbilityWriteSensitive, rt.handleTestObservabilityExport))
}
