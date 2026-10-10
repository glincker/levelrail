package api

import "net/http"

// registerAppImportRoutes registers the guided app import flow. Every
// route that carries or uses the source credential is AbilityWriteSensitive.
func (rt *Router) registerAppImportRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/migration/apps/plan", rt.requireAbility(AbilityWriteSensitive, rt.handleAppImportPlan))
	mux.HandleFunc("POST /api/v1/migration/apps/sessions", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateAppImportSession))
	mux.HandleFunc("GET /api/v1/migration/apps/sessions", rt.requireAbility(AbilityRead, rt.handleListAppImportSessions))
	mux.HandleFunc("GET /api/v1/migration/apps/sessions/{id}", rt.requireAbility(AbilityRead, rt.handleGetAppImportSession))
	mux.HandleFunc("DELETE /api/v1/migration/apps/sessions/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteAppImportSession))
	mux.HandleFunc("POST /api/v1/migration/apps/sessions/{id}/connect", rt.requireAbility(AbilityWriteSensitive, rt.handleConnectAppImportSession))
	mux.HandleFunc("PUT /api/v1/migration/apps/sessions/{id}/plan", rt.requireAbility(AbilityWriteSensitive, rt.handlePutAppImportPlan))
	mux.HandleFunc("POST /api/v1/migration/apps/sessions/{id}/stage", rt.requireAbility(AbilityWriteSensitive, rt.handleStageAppImport))
	mux.HandleFunc("POST /api/v1/migration/apps/sessions/{id}/verify", rt.requireAbility(AbilityWriteSensitive, rt.handleVerifyAppImport))
	mux.HandleFunc("POST /api/v1/migration/apps/sessions/{id}/rollback", rt.requireAbility(AbilityWriteSensitive, rt.handleRollbackAppImport))
	mux.HandleFunc("GET /api/v1/migration/apps/sessions/{id}/images", rt.requireAbility(AbilityRead, rt.handleAppImportImages))
	mux.HandleFunc("GET /api/v1/migration/apps/sessions/{id}/images/status", rt.requireAbility(AbilityRead, rt.handleAppImportImagesStatus))
	mux.HandleFunc("POST /api/v1/migration/apps/sessions/{id}/images/transfer", rt.requireAbility(AbilityWriteSensitive, rt.handleTransferAppImportImages))
	mux.HandleFunc("POST /api/v1/migration/apps/sessions/{id}/images/cancel", rt.requireAbility(AbilityWriteSensitive, rt.handleCancelAppImportImages))
	mux.HandleFunc("GET /api/v1/migration/apps/sessions/{id}/volumes", rt.requireAbility(AbilityRead, rt.handleAppImportVolumes))
	mux.HandleFunc("PUT /api/v1/migration/apps/sessions/{id}/items/{item}/volumes/{vol}", rt.requireAbility(AbilityWriteSensitive, rt.handleSetAppImportVolume))
	mux.HandleFunc("GET /api/v1/migration/apps/sessions/{id}/cutover", rt.requireAbility(AbilityRead, rt.handleAppImportCutover))
	mux.HandleFunc("GET /api/v1/migration/apps/sessions/{id}/cutover/verify", rt.requireAbility(AbilityWriteSensitive, rt.handleAppImportCutoverVerify))
	mux.HandleFunc("POST /api/v1/migration/apps/sessions/{id}/items/{item}/route", rt.requireAbility(AbilityWriteSensitive, rt.handleRouteAppImport))
	mux.HandleFunc("GET /api/v1/migration/apps/sessions/{id}/receipt", rt.requireAbility(AbilityRead, rt.handleAppImportReceipt))
}
