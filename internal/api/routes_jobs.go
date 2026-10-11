package api

import "net/http"

// registerJobRoutes serves scheduled jobs under /api/v1/apps/{name}/jobs and
// the legacy /scheduled-tasks alias with the same handlers. Reads need
// AbilityRead, config changes AbilityWrite, and anything that runs a
// command right now (run) AbilityDeploy, matching restart and stop.
func (rt *Router) registerJobRoutes(mux *http.ServeMux) {
	for _, base := range []string{"/api/v1/apps/{name}/jobs", "/api/v1/apps/{name}/scheduled-tasks"} {
		mux.HandleFunc("POST "+base, rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleCreateScheduledTask))
		mux.HandleFunc("GET "+base, rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListScheduledTasks))
		mux.HandleFunc("GET "+base+"/{id}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetScheduledTask))
		mux.HandleFunc("PUT "+base+"/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleUpdateScheduledTask))
		mux.HandleFunc("DELETE "+base+"/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeleteScheduledTask))
		mux.HandleFunc("POST "+base+"/{id}/run", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleRunScheduledTaskNow))
		mux.HandleFunc("POST "+base+"/{id}/enable", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSetScheduledTaskEnabled(true)))
		mux.HandleFunc("POST "+base+"/{id}/disable", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSetScheduledTaskEnabled(false)))
		mux.HandleFunc("GET "+base+"/{id}/runs", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListScheduledTaskRuns))
		mux.HandleFunc("GET "+base+"/{id}/runs/{run}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetScheduledTaskRun))
	}
	mux.HandleFunc("GET /api/v1/apps/{name}/job-runs", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListScheduledTaskRuns))
}

// WithJobLimits sets the per-app job count cap (APP_JOBS_MAX_PER_APP).
func WithJobLimits(maxPerApp int) Option {
	return func(rt *Router) { rt.jobsMaxPerApp = maxPerApp }
}
