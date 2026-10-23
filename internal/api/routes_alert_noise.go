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
	mux.HandleFunc("GET /api/v1/alert-defaults", rt.requireAbility(AbilityRead, rt.handleListDefaultAlerts))
	mux.HandleFunc("POST /api/v1/alert-defaults/reset", rt.requireAbility(AbilityWrite, rt.handleResetDefaultAlerts))
	mux.HandleFunc("GET /api/v1/alert-defaults/cert-expiry/email", rt.requireAbility(AbilityRead, rt.handleGetCertExpiryEmail))
	mux.HandleFunc("PUT /api/v1/alert-defaults/cert-expiry/email", rt.requireAbility(AbilityWrite, rt.handleSetCertExpiryEmail))
	mux.HandleFunc("PUT /api/v1/alert-defaults/{id}", rt.requireAbility(AbilityWrite, rt.handleSetDefaultAlertEnabled))
	mux.HandleFunc("GET /api/v1/inbox", rt.requireAbility(AbilityRead, rt.handleListInbox))
	mux.HandleFunc("GET /api/v1/inbox/unread-count", rt.requireAbility(AbilityRead, rt.handleInboxUnreadCount))
	mux.HandleFunc("POST /api/v1/inbox/read", rt.requireAbility(AbilityWrite, rt.handleInboxMarkRead))
	mux.HandleFunc("GET /api/v1/inbox/state", rt.requireAbility(AbilityRead, rt.handleInboxState))
	mux.HandleFunc("POST /api/v1/inbox/archive", rt.requireAbility(AbilityWrite, rt.handleInboxArchive))
	mux.HandleFunc("POST /api/v1/inbox/delete", rt.requireAbility(AbilityWrite, rt.handleInboxDelete))
	mux.HandleFunc("POST /api/v1/inbox/clear", rt.requireAbility(AbilityWrite, rt.handleInboxClear))
	mux.HandleFunc("GET /api/v1/inbox/preferences", rt.requireAbility(AbilityRead, rt.handleGetInboxPrefs))
	mux.HandleFunc("PUT /api/v1/inbox/preferences", rt.requireAbility(AbilityWrite, rt.handlePutInboxPrefs))
}
