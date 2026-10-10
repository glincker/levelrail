package api

import "net/http"

// registerPlatformImportRoutes registers the import-from-another-platform
// routes. Both take the source credential in the body, so both are
// AbilityWriteSensitive even though discover changes nothing.
func (rt *Router) registerPlatformImportRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/imports/platform/discover", rt.requireAbility(AbilityWriteSensitive, rt.handleDiscoverPlatformImport))
	mux.HandleFunc("POST /api/v1/imports/platform/apply", rt.requireAbility(AbilityWriteSensitive, rt.handleApplyPlatformImport))
	mux.HandleFunc("GET /api/v1/imports/platform/databases", rt.requireAbility(AbilityRead, rt.handleListDatabaseDataCopies))
	mux.HandleFunc("GET /api/v1/imports/platform/databases/{name}", rt.requireAbility(AbilityRead, rt.handleGetDatabaseDataCopy))
	mux.HandleFunc("POST /api/v1/imports/platform/databases/{name}/copy", rt.requireAbility(AbilityWriteSensitive, rt.handleCopyDatabaseData))
	mux.HandleFunc("GET /api/v1/migration/volumes", rt.requireAbility(AbilityRead, rt.handleVolumeGuide))
	mux.HandleFunc("GET /api/v1/migration/cutover", rt.requireAbility(AbilityRead, rt.handleCutoverReport))
	mux.HandleFunc("GET /api/v1/migration/cutover/verify", rt.requireAbility(AbilityWriteSensitive, rt.handleCutoverVerify))
	mux.HandleFunc("GET /api/v1/migration/hub/local-sources", rt.requireAbility(AbilityRead, rt.handleHubLocalSources))
	mux.HandleFunc("POST /api/v1/migration/hub/sessions", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateHubSession))
	mux.HandleFunc("GET /api/v1/migration/hub/sessions", rt.requireAbility(AbilityRead, rt.handleListHubSessions))
	mux.HandleFunc("GET /api/v1/migration/hub/sessions/{id}", rt.requireAbility(AbilityRead, rt.handleGetHubSession))
	mux.HandleFunc("DELETE /api/v1/migration/hub/sessions/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteHubSession))
	mux.HandleFunc("PUT /api/v1/migration/hub/sessions/{id}/selection", rt.requireAbility(AbilityWriteSensitive, rt.handlePutHubSelection))
	mux.HandleFunc("PUT /api/v1/migration/hub/sessions/{id}/step", rt.requireAbility(AbilityWriteSensitive, rt.handlePutHubStep))
	mux.HandleFunc("POST /api/v1/migration/hub/sessions/{id}/apply", rt.requireAbility(AbilityWriteSensitive, rt.handleApplyHubSession))
	mux.HandleFunc("GET /api/v1/migration/hub/sessions/{id}/receipt", rt.requireAbility(AbilityRead, rt.handleHubReceipt))
	mux.HandleFunc("GET /api/v1/migration/hub/sessions/{id}/items/{db}/connection", rt.requireAbility(AbilityRead, rt.handleHubConnection))
	mux.HandleFunc("POST /api/v1/migration/hub/sessions/{id}/items/{db}/reveal", rt.requireAbility(AbilityReadSensitive, rt.handleHubReveal))
}
