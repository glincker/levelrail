package api

import "net/http"

// registerPlatformImportRoutes registers the import-from-another-platform
// routes. Both take the source credential in the body, so both are
// AbilityWriteSensitive even though discover changes nothing.
func (rt *Router) registerPlatformImportRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/imports/platform/discover", rt.requireAbility(AbilityWriteSensitive, rt.handleDiscoverPlatformImport))
	mux.HandleFunc("POST /api/v1/imports/platform/apply", rt.requireAbility(AbilityWriteSensitive, rt.handleApplyPlatformImport))
}
