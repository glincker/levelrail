package api

import "net/http"

func (rt *Router) registerMonitorRoutes(mux *http.ServeMux) {
	// List uptime monitors. Answers 404 unless APP_MONITORS_ENABLED is set.
	mux.HandleFunc("GET /api/v1/monitors", rt.requireAbilityForResource(AbilityRead, monitorsResource, rt.handleListMonitors))
	// Create an http or tcp uptime monitor. Header values are stored encrypted and never returned.
	mux.HandleFunc("POST /api/v1/monitors", rt.requireAbilityForResource(AbilityWrite, monitorsResource, rt.handleCreateMonitor))
	// Change a monitor's status page link: open and resolve an incident on its component when it fails.
	mux.HandleFunc("PATCH /api/v1/monitors/{id}", rt.requireAbilityForResource(AbilityWrite, monitorsResource, rt.handleUpdateMonitorLink))
	// Delete a monitor with its results and rollups.
	mux.HandleFunc("DELETE /api/v1/monitors/{id}", rt.requireAbilityForResource(AbilityWrite, monitorsResource, rt.handleDeleteMonitor))
	// Recent raw results and hourly rollups for a monitor (window and limit query parameters).
	mux.HandleFunc("GET /api/v1/monitors/{id}/results", rt.requireAbilityForResource(AbilityRead, monitorsResource, rt.handleMonitorResults))
	// Replace a heartbeat monitor's secret ping URL; the old one stops working at once and the new one is shown once.
	mux.HandleFunc("POST /api/v1/monitors/{id}/rotate-token", rt.requireAbilityForResource(AbilityWrite, monitorsResource, rt.handleRotateHeartbeat))
	// Heartbeat ping for a cron monitor, authenticated only by the secret token in the path. Answers 204 or a uniform 404.
	mux.HandleFunc("GET /api/v1/heartbeat/{token}", rt.handleHeartbeatPing)
	// Heartbeat ping for a cron monitor, authenticated only by the secret token in the path. Answers 204 or a uniform 404.
	mux.HandleFunc("POST /api/v1/heartbeat/{token}", rt.handleHeartbeatPing)
	// Heartbeat start or fail signal; a fail marks the monitor down at once.
	mux.HandleFunc("GET /api/v1/heartbeat/{token}/{signal}", rt.handleHeartbeatPing)
	// Heartbeat start or fail signal; a fail marks the monitor down at once.
	mux.HandleFunc("POST /api/v1/heartbeat/{token}/{signal}", rt.handleHeartbeatPing)
}
