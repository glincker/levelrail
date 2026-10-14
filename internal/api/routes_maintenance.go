package api

import "net/http"

// registerScheduledMaintenanceRoutes wires scheduled maintenance windows and the
// read-only change calendar.
func (rt *Router) registerScheduledMaintenanceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/scheduled-maintenance", rt.requireAbility(AbilityRead, rt.handleListScheduledMaintenance))
	mux.HandleFunc("POST /api/v1/scheduled-maintenance", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateScheduledMaintenance))
	mux.HandleFunc("DELETE /api/v1/scheduled-maintenance/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteScheduledMaintenance))
}
