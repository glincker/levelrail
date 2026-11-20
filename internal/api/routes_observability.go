package api

import "net/http"

// registerObservabilityRoutes wires the provider presets and the exports
// made from them. Exports ship instance-wide data, so managing them is root only like data export; the catalog is a
// plain read.
func (rt *Router) registerObservabilityRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/observability/presets", rt.requireAbility(AbilityRead, rt.handleListObservabilityPresets))
	mux.HandleFunc("GET /api/v1/observability/exports", rt.requireAbility(AbilityRoot, rt.handleListObservabilityExports))
	mux.HandleFunc("POST /api/v1/observability/exports", rt.requireAbility(AbilityRoot, rt.handleCreateObservabilityExport))
	mux.HandleFunc("DELETE /api/v1/observability/exports/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteObservabilityExport))
	mux.HandleFunc("POST /api/v1/observability/exports/{id}/test", rt.requireAbility(AbilityRoot, rt.handleTestObservabilityExport))
}
