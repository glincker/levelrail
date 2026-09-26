package api

import "net/http"

func (rt *Router) registerStatusPageRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /public/status", rt.handlePublicStatusHTML)
	mux.HandleFunc("GET /public/status.json", rt.handlePublicStatusJSON)
	mux.HandleFunc("GET /public/status.rss", rt.handlePublicStatusRSS)

	mux.HandleFunc("GET /api/v1/status-page", rt.requireAbilityForResource(AbilityRead, statusPageResource, rt.handleGetStatusPage))
	mux.HandleFunc("PUT /api/v1/status-page", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handlePutStatusPage))
	mux.HandleFunc("GET /api/v1/status-page/preview", rt.requireAbilityForResource(AbilityRead, statusPageResource, rt.handleStatusPagePreview))
	mux.HandleFunc("GET /api/v1/status-page/components", rt.requireAbilityForResource(AbilityRead, statusPageResource, rt.handleListStatusComponents))
	mux.HandleFunc("POST /api/v1/status-page/components", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleCreateStatusComponent))
	mux.HandleFunc("PUT /api/v1/status-page/components/{id}", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleUpdateStatusComponent))
	mux.HandleFunc("DELETE /api/v1/status-page/components/{id}", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleDeleteStatusComponent))
	mux.HandleFunc("GET /api/v1/status-page/incidents", rt.requireAbilityForResource(AbilityRead, statusPageResource, rt.handleListStatusIncidents))
	mux.HandleFunc("POST /api/v1/status-page/incidents", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleCreateStatusIncident))
	mux.HandleFunc("POST /api/v1/status-page/incidents/{id}/updates", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handlePostStatusIncidentUpdate))
	mux.HandleFunc("DELETE /api/v1/status-page/incidents/{id}", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleDeleteStatusIncident))
}
