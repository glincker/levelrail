package api

import "net/http"

func (rt *Router) registerStatusPageRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /public/status", rt.handlePublicStatusHTML)
	mux.HandleFunc("GET /public/status.json", rt.handlePublicStatusJSON)
	mux.HandleFunc("GET /public/status.rss", rt.handlePublicStatusRSS)
	mux.HandleFunc("GET /public/status.atom", rt.handlePublicStatusAtom)
	mux.HandleFunc("GET /public/status/history", rt.handlePublicStatusHistory)
	mux.HandleFunc("GET /public/status/incidents/{id}", rt.handlePublicStatusIncident)
	mux.HandleFunc("GET /public/status/badge.svg", rt.handlePublicStatusBadge)

	mux.HandleFunc("GET /api/v1/status-page", rt.requireAbilityForResource(AbilityRead, statusPageResource, rt.handleGetStatusPage))
	mux.HandleFunc("PUT /api/v1/status-page", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handlePutStatusPage))
	mux.HandleFunc("GET /api/v1/status-page/preview", rt.requireAbilityForResource(AbilityRead, statusPageResource, rt.handleStatusPagePreview))
	mux.HandleFunc("GET /api/v1/status-page/components", rt.requireAbilityForResource(AbilityRead, statusPageResource, rt.handleListStatusComponents))
	mux.HandleFunc("POST /api/v1/status-page/components", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleCreateStatusComponent))
	mux.HandleFunc("PUT /api/v1/status-page/components/{id}", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleUpdateStatusComponent))
	mux.HandleFunc("DELETE /api/v1/status-page/components/{id}", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleDeleteStatusComponent))
	mux.HandleFunc("GET /api/v1/status-page/groups", rt.requireAbilityForResource(AbilityRead, statusPageResource, rt.handleListStatusGroups))
	mux.HandleFunc("POST /api/v1/status-page/groups", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleCreateStatusGroup))
	mux.HandleFunc("PUT /api/v1/status-page/groups/{id}", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleUpdateStatusGroup))
	mux.HandleFunc("DELETE /api/v1/status-page/groups/{id}", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleDeleteStatusGroup))
	mux.HandleFunc("GET /api/v1/status-page/incidents", rt.requireAbilityForResource(AbilityRead, statusPageResource, rt.handleListStatusIncidents))
	mux.HandleFunc("POST /api/v1/status-page/incidents", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleCreateStatusIncident))
	mux.HandleFunc("POST /api/v1/status-page/incidents/{id}/updates", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handlePostStatusIncidentUpdate))
	mux.HandleFunc("DELETE /api/v1/status-page/incidents/{id}", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleDeleteStatusIncident))
}
