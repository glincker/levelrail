package api

import "net/http"

// registerPipelineRoutes mounts the CI/CD pipeline endpoints. Reads are
// AbilityRead, editing definitions is AbilityWrite, and anything that starts,
// cancels, or approves a run is AbilityDeploy since a run can build and
// deploy. Approval gates additionally enforce their own required ability.
func (rt *Router) registerPipelineRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/pipelines", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListPipelines))
	mux.HandleFunc("POST /api/v1/apps/{name}/pipelines", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleCreatePipeline))
	mux.HandleFunc("GET /api/v1/apps/{name}/pipelines/{pname}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetPipeline))
	mux.HandleFunc("PUT /api/v1/apps/{name}/pipelines/{pname}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleUpdatePipeline))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/pipelines/{pname}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeletePipeline))
	mux.HandleFunc("POST /api/v1/apps/{name}/pipelines/{pname}/runs", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleStartPipelineRun))
	mux.HandleFunc("GET /api/v1/apps/{name}/pipeline-runs", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListPipelineRuns))
	mux.HandleFunc("GET /api/v1/apps/{name}/pipeline-runs/{id}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetPipelineRun))
	mux.HandleFunc("GET /api/v1/apps/{name}/pipeline-runs/{id}/logs", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListPipelineRunLogs))
	mux.HandleFunc("GET /api/v1/apps/{name}/pipeline-runs/{id}/logs/stream", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.withStreamReauth(appResourceFromPath, rt.handleStreamPipelineRunLogs)))
	mux.HandleFunc("POST /api/v1/apps/{name}/pipeline-runs/{id}/cancel", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleCancelPipelineRun))
	mux.HandleFunc("POST /api/v1/apps/{name}/pipeline-runs/{id}/rerun", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleRerunPipelineRun))
	mux.HandleFunc("POST /api/v1/apps/{name}/pipeline-runs/{id}/approvals/{approval}", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDecidePipelineApproval))
	mux.HandleFunc("POST /api/v1/apps/{name}/pipeline-runs/{id}/hold", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDecidePipelineRunHold))
	mux.HandleFunc("GET /api/v1/apps/{name}/pipeline-triggers", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListPipelineTriggers))
	mux.HandleFunc("GET /api/v1/apps/{name}/pipeline-sync", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetPipelineSync))
	mux.HandleFunc("PUT /api/v1/apps/{name}/pipeline-sync", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSetPipelineSync))
	mux.HandleFunc("POST /api/v1/apps/{name}/pipeline-sync", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleRunPipelineSync))
	mux.HandleFunc("POST /api/v1/pipelines/validate", rt.requireAbility(AbilityRead, rt.handleValidatePipeline))
	mux.HandleFunc("GET /api/v1/pipelines/schema", rt.requireAbility(AbilityRead, rt.handlePipelineSchema))
}

// WithPipelines enables the pipeline endpoints. runner may be nil, in which
// case definitions can be edited but starting, cancelling, and re-running
// return 501.
func WithPipelines(st PipelineStore, runner PipelineRunner) Option {
	return func(rt *Router) {
		rt.pipelineStore = st
		rt.pipelineRunner = runner
	}
}
