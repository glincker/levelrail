package api

import "net/http"

// registerPipelineOverviewRoutes mounts the cross-app pipeline reads. The
// route needs read; each row is then checked against the caller's per-app
// policies.
func (rt *Router) registerPipelineOverviewRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/pipeline-runs", rt.requireAbility(AbilityRead, rt.handleListAllPipelineRuns))
	mux.HandleFunc("GET /api/v1/pipelines/summary", rt.requireAbility(AbilityRead, rt.handleGetPipelineSummary))
}
