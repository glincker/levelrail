package api

import "net/http"

// registerNextBuildReportRoutes registers the Next.js build report route.
func (rt *Router) registerNextBuildReportRoutes(mux *http.ServeMux) {
	// Next.js route table and warnings parsed from one deploy attempt's stored build log.
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys/{deployId}/build-report", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleNextBuildReport))
}
