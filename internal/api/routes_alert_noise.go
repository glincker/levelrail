package api

import "net/http"

func (rt *Router) registerAlertNoiseRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/alert-silences", rt.requireAbility(AbilityRead, rt.handleListAlertSilences))
	mux.HandleFunc("POST /api/v1/alert-silences", rt.requireAbility(AbilityWrite, rt.handleCreateAlertSilence))
	mux.HandleFunc("DELETE /api/v1/alert-silences/{id}", rt.requireAbility(AbilityWrite, rt.handleExpireAlertSilence))
	mux.HandleFunc("GET /api/v1/alert-maintenance-windows", rt.requireAbility(AbilityRead, rt.handleListMaintenanceWindows))
	mux.HandleFunc("POST /api/v1/alert-maintenance-windows", rt.requireAbility(AbilityWrite, rt.handleCreateMaintenanceWindow))
	mux.HandleFunc("PUT /api/v1/alert-maintenance-windows/{id}", rt.requireAbility(AbilityWrite, rt.handleUpdateMaintenanceWindow))
	mux.HandleFunc("DELETE /api/v1/alert-maintenance-windows/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteMaintenanceWindow))
	mux.HandleFunc("GET /api/v1/alert-history", rt.requireAbility(AbilityRead, rt.handleListAlertHistory))
	mux.HandleFunc("POST /api/v1/apps/{name}/alerts/{id}/silence", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSilenceAlertRule))
	mux.HandleFunc("GET /api/v1/apps/{name}/alert-history", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListAppAlertHistory))
}
