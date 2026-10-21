package api

import "net/http"

// registerDeployGateRoutes wires the readiness gate timeline, per-app gate
// settings and pinned rollback targets.
func (rt *Router) registerDeployGateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys/{deployId}/gate", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDeployGate))
	mux.HandleFunc("GET /api/v1/apps/{name}/deploy-safety", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDeploySafety))
	// Deploy tier, like auto-rollback: these thresholds decide when a
	// release takes traffic and when it is rolled back.
	mux.HandleFunc("PUT /api/v1/apps/{name}/deploy-safety", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handlePutDeploySafety))
	mux.HandleFunc("GET /api/v1/apps/{name}/watch", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppWatch))
	mux.HandleFunc("POST /api/v1/apps/{name}/watch/stop", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleStopAppWatch))
	mux.HandleFunc("GET /api/v1/apps/{name}/rollback-targets", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListRollbackTargets))
}
