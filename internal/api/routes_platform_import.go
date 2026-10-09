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
}
