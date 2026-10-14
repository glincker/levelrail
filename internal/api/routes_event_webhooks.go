package api

import "net/http"

// registerEventWebhookRoutes wires outbound event webhook management, its
// delivery log and replay. Management is root only.
func (rt *Router) registerEventWebhookRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/event-webhooks", rt.requireAbility(AbilityRoot, rt.handleListEventWebhooks))
	mux.HandleFunc("POST /api/v1/event-webhooks", rt.requireAbility(AbilityRoot, rt.handleCreateEventWebhook))
	mux.HandleFunc("GET /api/v1/event-webhook-types", rt.requireAbility(AbilityRoot, rt.handleListEventWebhookTypes))
	mux.HandleFunc("GET /api/v1/event-webhooks/{id}", rt.requireAbility(AbilityRoot, rt.handleGetEventWebhook))
	mux.HandleFunc("PATCH /api/v1/event-webhooks/{id}", rt.requireAbility(AbilityRoot, rt.handleUpdateEventWebhook))
	mux.HandleFunc("DELETE /api/v1/event-webhooks/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteEventWebhook))
	mux.HandleFunc("POST /api/v1/event-webhooks/{id}/test", rt.requireAbility(AbilityRoot, rt.handleTestEventWebhook))
	mux.HandleFunc("POST /api/v1/event-webhooks/{id}/rotate-secret", rt.requireAbility(AbilityRoot, rt.handleRotateEventWebhookSecret))
	mux.HandleFunc("GET /api/v1/event-webhooks/{id}/deliveries", rt.requireAbility(AbilityRoot, rt.handleListEventWebhookDeliveries))
	mux.HandleFunc("POST /api/v1/event-webhook-deliveries/{id}/replay", rt.requireAbility(AbilityRoot, rt.handleReplayEventWebhookDelivery))
}
