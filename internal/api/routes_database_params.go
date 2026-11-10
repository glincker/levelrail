package api

import "net/http"

// registerDatabaseParamsRoutes mounts the engine parameters editor. Staging is
// a desired-state edit; apply restarts the database, so it is the sensitive tier.
func (rt *Router) registerDatabaseParamsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/databases/{name}/params", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleGetDatabaseParams))
	mux.HandleFunc("PUT /api/v1/databases/{name}/params", rt.requireAbilityForResource(AbilityWrite, databaseResourceFromPath, rt.handleStageDatabaseParams))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/params/pending", rt.requireAbilityForResource(AbilityWrite, databaseResourceFromPath, rt.handleDiscardDatabaseParams))
	mux.HandleFunc("POST /api/v1/databases/{name}/params/apply", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleApplyDatabaseParams))
}
