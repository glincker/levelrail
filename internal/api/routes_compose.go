package api

import "net/http"

// registerComposeRoutes wires compose stacks: status, history, rollback,
// redeploy, teardown, the import preview and template installs.
// POST /api/v1/apps/{name}/compose (deploy) lives with the core app routes.
func (rt *Router) registerComposeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/compose", rt.requireAbility(AbilityRead, rt.handleListComposeStacks))
	// Parse only, nothing saved; deploy tier so the endpoint is not a free YAML parser for read-only tokens.
	mux.HandleFunc("POST /api/v1/compose/preview", rt.requireAbility(AbilityDeploy, rt.handlePreviewCompose))
	mux.HandleFunc("GET /api/v1/apps/{name}/compose", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetComposeStack))
	mux.HandleFunc("GET /api/v1/apps/{name}/compose/revisions", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListComposeRevisions))
	mux.HandleFunc("GET /api/v1/apps/{name}/compose/revisions/{revision}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetComposeRevision))
	mux.HandleFunc("GET /api/v1/apps/{name}/template-upgrade", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetTemplateUpgrade))
	mux.HandleFunc("POST /api/v1/apps/{name}/template-upgrade", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleApplyTemplateUpgrade))
	mux.HandleFunc("GET /api/v1/apps/{name}/template-upgrade/run", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetTemplateUpgradeRun))
	mux.HandleFunc("POST /api/v1/apps/{name}/compose/rollback", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleRollbackCompose))
	mux.HandleFunc("POST /api/v1/apps/{name}/compose/redeploy", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleRedeployCompose))
	mux.HandleFunc("POST /api/v1/apps/{name}/compose/services/{service}/restart", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.composeServiceAction(rt.handleRestartApp)))
	mux.HandleFunc("POST /api/v1/apps/{name}/compose/services/{service}/stop", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.composeServiceAction(rt.handleStopApp)))
	mux.HandleFunc("POST /api/v1/apps/{name}/compose/services/{service}/start", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.composeServiceAction(rt.handleStartApp)))
	mux.HandleFunc("GET /api/v1/apps/{name}/compose/services/{service}/logs", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.composeServiceAction(rt.handleQueryLogs)))
	mux.HandleFunc("POST /api/v1/service-templates/{id}/install", rt.requireAbility(AbilityDeploy, rt.handleInstallServiceTemplate))
	// Same tier as DELETE /apps/{name}; ?volumes=true also needs write-sensitive, checked in the handler.
	mux.HandleFunc("DELETE /api/v1/apps/{name}/compose", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleComposeDown))
}
