package api

import "net/http"

// Explore filters rows by the caller's visibility like the usage report, so
// tenants may call it. Budgets span the whole instance and stay closed to them.
func init() {
	tenantRoutes["GET /api/v1/costs/explore"] = true
}

func (rt *Router) registerCostExplorerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/costs/explore", rt.requireAbility(AbilityRead, rt.handleCostExplore))
	mux.HandleFunc("GET /api/v1/costs/app-caps", rt.requireAbility(AbilityRead, rt.handleListSpendCaps))
	mux.HandleFunc("GET /api/v1/costs/budgets", rt.requireAbility(AbilityRead, rt.handleListCostBudgets))
	mux.HandleFunc("POST /api/v1/costs/budgets", rt.requireAbility(AbilityRoot, rt.handleCreateCostBudget))
	mux.HandleFunc("PUT /api/v1/costs/budgets/{id}", rt.requireAbility(AbilityRoot, rt.handleUpdateCostBudget))
	mux.HandleFunc("DELETE /api/v1/costs/budgets/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteCostBudget))
}
