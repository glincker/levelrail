package api

import "net/http"

// registerAppImportRoutes registers the guided app import flow. Every
// route that carries or uses the source credential is AbilityWriteSensitive.
func (rt *Router) registerAppImportRoutes(mux *http.ServeMux) {
	const base = "/api/v1/migration/apps"
	mux.HandleFunc("POST "+base+"/plan", rt.requireAbility(AbilityWriteSensitive, rt.handleAppImportPlan))
	mux.HandleFunc("POST "+base+"/sessions", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateAppImportSession))
	mux.HandleFunc("GET "+base+"/sessions", rt.requireAbility(AbilityRead, rt.handleListAppImportSessions))
	mux.HandleFunc("GET "+base+"/sessions/{id}", rt.requireAbility(AbilityRead, rt.handleGetAppImportSession))
	mux.HandleFunc("DELETE "+base+"/sessions/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteAppImportSession))
	mux.HandleFunc("POST "+base+"/sessions/{id}/connect", rt.requireAbility(AbilityWriteSensitive, rt.handleConnectAppImportSession))
	mux.HandleFunc("PUT "+base+"/sessions/{id}/plan", rt.requireAbility(AbilityWriteSensitive, rt.handlePutAppImportPlan))
	mux.HandleFunc("POST "+base+"/sessions/{id}/stage", rt.requireAbility(AbilityWriteSensitive, rt.handleStageAppImport))
	mux.HandleFunc("POST "+base+"/sessions/{id}/verify", rt.requireAbility(AbilityWriteSensitive, rt.handleVerifyAppImport))
	mux.HandleFunc("POST "+base+"/sessions/{id}/rollback", rt.requireAbility(AbilityWriteSensitive, rt.handleRollbackAppImport))
	mux.HandleFunc("GET "+base+"/sessions/{id}/volumes", rt.requireAbility(AbilityRead, rt.handleAppImportVolumes))
	mux.HandleFunc("PUT "+base+"/sessions/{id}/items/{item}/volumes/{vol}", rt.requireAbility(AbilityWriteSensitive, rt.handleSetAppImportVolume))
	mux.HandleFunc("GET "+base+"/sessions/{id}/cutover", rt.requireAbility(AbilityRead, rt.handleAppImportCutover))
	mux.HandleFunc("GET "+base+"/sessions/{id}/cutover/verify", rt.requireAbility(AbilityWriteSensitive, rt.handleAppImportCutoverVerify))
	mux.HandleFunc("POST "+base+"/sessions/{id}/items/{item}/route", rt.requireAbility(AbilityWriteSensitive, rt.handleRouteAppImport))
	mux.HandleFunc("GET "+base+"/sessions/{id}/receipt", rt.requireAbility(AbilityRead, rt.handleAppImportReceipt))
}
