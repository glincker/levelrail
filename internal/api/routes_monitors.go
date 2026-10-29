package api

import "net/http"

func (rt *Router) registerMonitorRoutes(mux *http.ServeMux) {
	// List uptime monitors. Answers 404 unless APP_MONITORS_ENABLED is set.
	mux.HandleFunc("GET /api/v1/monitors", rt.requireAbilityForResource(AbilityRead, monitorsResource, rt.handleListMonitors))
	// Create an http or tcp uptime monitor. Header values are stored encrypted and never returned.
	mux.HandleFunc("POST /api/v1/monitors", rt.requireAbilityForResource(AbilityWrite, monitorsResource, rt.handleCreateMonitor))
	// Delete a monitor with its results and rollups.
	mux.HandleFunc("DELETE /api/v1/monitors/{id}", rt.requireAbilityForResource(AbilityWrite, monitorsResource, rt.handleDeleteMonitor))
	// Recent raw results and hourly rollups for a monitor (window and limit query parameters).
	mux.HandleFunc("GET /api/v1/monitors/{id}/results", rt.requireAbilityForResource(AbilityRead, monitorsResource, rt.handleMonitorResults))
}
