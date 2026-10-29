package api

import "net/http"

// registerRolloutRoutes wires an app's rollout policy and live wave progress.
func (rt *Router) registerRolloutRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/rollout", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetRollout))
}
