package api

import "net/http"

// registerLogExplorerRoutes registers saved log queries and error signature
// aggregation. Saved queries are per caller (or shared by an AbilityWrite
// holder); signatures reuse the app read boundary of /apps/{name}/logs.
func (rt *Router) registerLogExplorerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/logs/queries", rt.requireAbility(AbilityRead, rt.handleListSavedLogQueries))
	mux.HandleFunc("POST /api/v1/logs/queries", rt.requireAbility(AbilityRead, rt.handleCreateSavedLogQuery))
	mux.HandleFunc("PUT /api/v1/logs/queries/{id}", rt.requireAbility(AbilityRead, rt.handleUpdateSavedLogQuery))
	mux.HandleFunc("DELETE /api/v1/logs/queries/{id}", rt.requireAbility(AbilityRead, rt.handleDeleteSavedLogQuery))
	mux.HandleFunc("GET /api/v1/apps/{name}/logs/signatures", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleLogSignatures))
}
