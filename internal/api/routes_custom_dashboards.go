package api

import "net/http"

func (rt *Router) registerCustomDashboardRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/dashboards", rt.requireAbility(AbilityRead, rt.handleListDashboards))
	mux.HandleFunc("GET /api/v1/dashboards/templates", rt.requireAbility(AbilityRead, rt.handleDashboardTemplates))
	mux.HandleFunc("POST /api/v1/dashboards/import", rt.requireAbility(AbilityWrite, rt.handleImportDashboard))
	mux.HandleFunc("POST /api/v1/dashboards", rt.requireAbility(AbilityWrite, rt.handleCreateDashboard))
	mux.HandleFunc("GET /api/v1/dashboards/{id}", rt.requireAbility(AbilityRead, rt.handleGetDashboard))
	mux.HandleFunc("GET /api/v1/dashboards/{id}/export", rt.requireAbility(AbilityRead, rt.handleExportDashboard))
	mux.HandleFunc("PUT /api/v1/dashboards/{id}", rt.requireAbility(AbilityWrite, rt.handleUpdateDashboard))
	mux.HandleFunc("DELETE /api/v1/dashboards/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteDashboard))
}
