package api

import "net/http"

// Reads need AbilityReadSensitive, like the git connection list. Binding writes
// also check, in the handler, that the caller can read the connection and that
// its scope covers the target.
func (rt *Router) registerConnectionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/connections", rt.requireAbility(AbilityReadSensitive, rt.handleListConnections))
	mux.HandleFunc("GET /api/v1/connections/catalog", rt.requireAbility(AbilityRead, rt.handleConnectionCatalog))
	mux.HandleFunc("GET /api/v1/connections/{ref}", rt.requireAbility(AbilityReadSensitive, rt.handleGetConnection))
	mux.HandleFunc("PUT /api/v1/connections/{ref}/meta", rt.requireAbility(AbilityWriteSensitive, rt.handleSetConnectionMeta))
	mux.HandleFunc("GET /api/v1/apps/{name}/bindings", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListAppBindings))
	mux.HandleFunc("POST /api/v1/apps/{name}/bindings", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleCreateAppBinding))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/bindings/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeleteAppBinding))
	mux.HandleFunc("GET /api/v1/organizations/{id}/bindings", rt.requireAbility(AbilityRead, rt.handleListOrgBindings))
	mux.HandleFunc("POST /api/v1/organizations/{id}/bindings", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateOrgBinding))
	mux.HandleFunc("DELETE /api/v1/organizations/{id}/bindings/{bid}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteOrgBinding))
}
