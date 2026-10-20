package api

import "net/http"

// Report and budget routes filter by team themselves, so tenants may call
// them (see tenantRoutes in team_gate.go). Settings and node bills are the
// instance's own invoice and stay closed to them.
func init() {
	for _, k := range []string{
		"GET /api/v1/usage/report",
		"GET /api/v1/usage/budgets",
		"PUT /api/v1/usage/budgets",
		"DELETE /api/v1/usage/budgets/{id}",
	} {
		tenantRoutes[k] = true
	}
}

// registerUsageCostRoutes: the report needs AbilityRead and is filtered to
// the caller's resources; budget writes need AbilityWrite plus a team admin
// role (or AbilityRoot) checked in the handler; prices need AbilityRoot.
func (rt *Router) registerUsageCostRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/usage/report", rt.requireAbility(AbilityRead, rt.handleUsageReport))
	mux.HandleFunc("GET /api/v1/usage/settings", rt.requireAbility(AbilityRoot, rt.handleGetUsageSettings))
	mux.HandleFunc("PUT /api/v1/usage/settings", rt.requireAbility(AbilityRoot, rt.handlePutUsageSettings))
	mux.HandleFunc("GET /api/v1/usage/budgets", rt.requireAbility(AbilityRead, rt.handleListUsageBudgets))
	mux.HandleFunc("PUT /api/v1/usage/budgets", rt.requireAbility(AbilityWrite, rt.handlePutUsageBudget))
	mux.HandleFunc("DELETE /api/v1/usage/budgets/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteUsageBudget))
}
