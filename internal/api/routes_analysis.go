package api

import "net/http"

func (rt *Router) registerAnalysisRoutes(mux *http.ServeMux) {
	// Read-only analysis reports (ADR 051, no model). AbilityRead, same
	// sensitivity as resource-recommendation; the by-id route is scoped to the report's app.
	mux.HandleFunc("GET /api/v1/apps/{name}/analysis/reports", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListAnalysisReports))
	// On-demand run for one app. AbilityWrite because it stores a report; 501 unless APP_ANALYSIS_ENABLED is on.
	mux.HandleFunc("POST /api/v1/apps/{name}/analysis/reports", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleRunAnalysisReport))
	mux.HandleFunc("GET /api/v1/analysis/reports/{id}", rt.requireAbilityForResource(AbilityRead, rt.analysisReportResource, rt.handleGetAnalysisReport))
}
