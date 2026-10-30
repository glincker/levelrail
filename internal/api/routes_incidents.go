package api

import "net/http"

// Internal incidents (ADR 061). Reads need read:sensitive because timelines carry probe detail;
// writes are audited by the shared write gate. All answer 501 unless incidents are enabled.
func (rt *Router) registerIncidentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/incidents", rt.requireAbilityForResource(AbilityReadSensitive, incidentsResource, rt.handleListIncidents))
	mux.HandleFunc("GET /api/v1/incidents/{id}", rt.requireAbilityForResource(AbilityReadSensitive, incidentsResource, rt.handleGetIncident))
	mux.HandleFunc("POST /api/v1/incidents", rt.requireAbilityForResource(AbilityWrite, incidentsResource, rt.handlePromoteIncident))
	mux.HandleFunc("POST /api/v1/incidents/{id}/ack", rt.requireAbilityForResource(AbilityWrite, incidentsResource, rt.handleAckIncident))
	mux.HandleFunc("POST /api/v1/incidents/{id}/resolve", rt.requireAbilityForResource(AbilityWrite, incidentsResource, rt.handleResolveIncident))
	mux.HandleFunc("POST /api/v1/incidents/{id}/notes", rt.requireAbilityForResource(AbilityWrite, incidentsResource, rt.handleAddIncidentNote))
}
