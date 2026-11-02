package api

import "net/http"

// registerAppGameRoutes wires the game server status ping. It only reads, so
// AbilityRead is enough; the probe targets the app's own loopback port.
func (rt *Router) registerAppGameRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/game-status", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppGameStatus))
}
