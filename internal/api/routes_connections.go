package api

import "net/http"

// Reads need AbilityReadSensitive, like the git connection list; the one write only touches sidecar metadata.
func (rt *Router) registerConnectionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/connections", rt.requireAbility(AbilityReadSensitive, rt.handleListConnections))
	mux.HandleFunc("GET /api/v1/connections/catalog", rt.requireAbility(AbilityRead, rt.handleConnectionCatalog))
	mux.HandleFunc("GET /api/v1/connections/{ref}", rt.requireAbility(AbilityReadSensitive, rt.handleGetConnection))
	mux.HandleFunc("PUT /api/v1/connections/{ref}/meta", rt.requireAbility(AbilityWriteSensitive, rt.handleSetConnectionMeta))
	mux.HandleFunc("GET /api/v1/apps/{name}/bindings", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListAppBindings))
}
