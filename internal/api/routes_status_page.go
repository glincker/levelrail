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
	mux.HandleFunc("GET /api/v1/status-page/webhooks", rt.requireAbilityForResource(AbilityRead, statusPageResource, rt.handleListStatusWebhooks))
	mux.HandleFunc("POST /api/v1/status-page/webhooks", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleCreateStatusWebhook))
	mux.HandleFunc("DELETE /api/v1/status-page/webhooks/{id}", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleDeleteStatusWebhook))
	mux.HandleFunc("POST /api/v1/status-page/webhooks/{id}/rotate", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleRotateStatusWebhook))
	mux.HandleFunc("POST /api/v1/status-page/webhooks/{id}/pause", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handlePauseStatusWebhook))
	mux.HandleFunc("POST /api/v1/status-page/webhooks/{id}/resume", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleResumeStatusWebhook))
	mux.HandleFunc("POST /api/v1/status-page/webhooks/{id}/test", rt.requireAbilityForResource(AbilityWrite, statusPageResource, rt.handleTestStatusWebhook))
	mux.HandleFunc("GET /api/v1/status-page/webhooks/{id}/deliveries", rt.requireAbilityForResource(AbilityRead, statusPageResource, rt.handleListStatusWebhookDeliveries))
}
