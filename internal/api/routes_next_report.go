package api

import "net/http"

// registerNextBuildReportRoutes registers the Next.js build report and advisor routes.
func (rt *Router) registerNextBuildReportRoutes(mux *http.ServeMux) {
	// Next.js route table and warnings parsed from one deploy attempt's stored build log.
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys/{deployId}/build-report", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleNextBuildReport))
	// Next.js checks of the saved app and its latest successful release, no build needed.
	mux.HandleFunc("GET /api/v1/apps/{name}/next-advisor", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleNextAdvisor))
}
