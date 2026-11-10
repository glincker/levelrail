package api

import "net/http"

// registerJobRoutes serves scheduled jobs under /api/v1/apps/{name}/jobs and
// the legacy /scheduled-tasks alias, plus one-off runs. Setting or running a
// command follows console exec access: AbilityRoot plus the app's exec setting.
// Patterns stay literal so the authz matrix and API reference can read them.
func (rt *Router) registerJobRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/jobs/runs", rt.requireAbilityForResource(AbilityRoot, oneShotAppFromBody, rt.handleStartOneShotJob))
	mux.HandleFunc("GET /api/v1/jobs/runs/{id}", rt.requireAbilityForResource(AbilityRead, rt.oneShotAppFromID, rt.handleGetOneShotJob))
	mux.HandleFunc("POST /api/v1/jobs/runs/{id}/cancel", rt.requireAbilityForResource(AbilityRoot, rt.oneShotAppFromID, rt.handleCancelOneShotJob))

	mux.HandleFunc("POST /api/v1/apps/{name}/jobs", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleCreateScheduledTask))
	mux.HandleFunc("GET /api/v1/apps/{name}/jobs", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListScheduledTasks))
	mux.HandleFunc("GET /api/v1/apps/{name}/jobs/{id}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetScheduledTask))
	mux.HandleFunc("PUT /api/v1/apps/{name}/jobs/{id}", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleUpdateScheduledTask))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/jobs/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeleteScheduledTask))
	mux.HandleFunc("POST /api/v1/apps/{name}/jobs/{id}/run", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleRunScheduledTaskNow))
	mux.HandleFunc("POST /api/v1/apps/{name}/jobs/{id}/enable", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleSetScheduledTaskEnabled(true)))
	mux.HandleFunc("POST /api/v1/apps/{name}/jobs/{id}/disable", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSetScheduledTaskEnabled(false)))
	mux.HandleFunc("GET /api/v1/apps/{name}/jobs/{id}/runs", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListScheduledTaskRuns))
	mux.HandleFunc("GET /api/v1/apps/{name}/jobs/{id}/runs/{run}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetScheduledTaskRun))

	mux.HandleFunc("POST /api/v1/apps/{name}/scheduled-tasks", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleCreateScheduledTask))
	mux.HandleFunc("GET /api/v1/apps/{name}/scheduled-tasks", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListScheduledTasks))
	mux.HandleFunc("GET /api/v1/apps/{name}/scheduled-tasks/{id}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetScheduledTask))
	mux.HandleFunc("PUT /api/v1/apps/{name}/scheduled-tasks/{id}", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleUpdateScheduledTask))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/scheduled-tasks/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeleteScheduledTask))
	mux.HandleFunc("POST /api/v1/apps/{name}/scheduled-tasks/{id}/run", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleRunScheduledTaskNow))
	mux.HandleFunc("POST /api/v1/apps/{name}/scheduled-tasks/{id}/enable", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleSetScheduledTaskEnabled(true)))
	mux.HandleFunc("POST /api/v1/apps/{name}/scheduled-tasks/{id}/disable", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSetScheduledTaskEnabled(false)))
	mux.HandleFunc("GET /api/v1/apps/{name}/scheduled-tasks/{id}/runs", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListScheduledTaskRuns))
	mux.HandleFunc("GET /api/v1/apps/{name}/scheduled-tasks/{id}/runs/{run}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetScheduledTaskRun))

	mux.HandleFunc("GET /api/v1/apps/{name}/job-runs", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListScheduledTaskRuns))
}

// WithJobLimits sets the per-app job count cap (APP_JOBS_MAX_PER_APP).
func WithJobLimits(maxPerApp int) Option {
	return func(rt *Router) { rt.jobsMaxPerApp = maxPerApp }
}
