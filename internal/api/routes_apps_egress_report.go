package api

import "net/http"

func (rt *Router) registerAppEgressReportRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/egress-report", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleAppEgressReport))
}
