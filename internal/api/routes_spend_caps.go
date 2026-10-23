package api

import "net/http"

// registerSpendCapRoutes: reading needs AbilityRead; changing or clearing a
// budget can pause an app, so it needs AbilityDeploy like other app settings.
func (rt *Router) registerSpendCapRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/budget", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetSpendCap))
	mux.HandleFunc("PUT /api/v1/apps/{name}/budget", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handlePutSpendCap))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/budget", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeleteSpendCap))
	mux.HandleFunc("POST /api/v1/apps/{name}/budget/dry-run", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleAppSpendDryRun))
	mux.HandleFunc("POST /api/v1/apps/{name}/budget/override", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleSpendOverride))
	mux.HandleFunc("POST /api/v1/costs/budgets/{id}/dry-run", rt.requireAbility(AbilityRead, rt.handleCostBudgetDryRun))
	mux.HandleFunc("GET /api/v1/apps/{name}/budget/history", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleSpendCapHistory))
}
