package api

import "net/http"

// registerAutoscaleRoutes: reading needs AbilityRead; changing or clearing a
// policy changes how many replicas run (and what they cost), so it needs
// AbilityDeploy like other app settings.
func (rt *Router) registerAutoscaleRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/autoscale", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAutoscale))
	mux.HandleFunc("PUT /api/v1/apps/{name}/autoscale", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handlePutAutoscale))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/autoscale", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeleteAutoscale))
	mux.HandleFunc("GET /api/v1/apps/{name}/autoscale/history", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleAutoscaleHistory))
}
