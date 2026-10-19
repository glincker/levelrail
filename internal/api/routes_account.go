package api

import "net/http"

// registerAccountSelfRoutes wires the caller's own account features. Every
// route resolves the account from the credential (accountCaller), never from
// the request, and checks its own ability level for tokens.
func (rt *Router) registerAccountSelfRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/auth/my-activity", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleMyActivity))
	mux.HandleFunc("GET /api/v1/auth/my-credentials", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleMyCredentials))
	mux.HandleFunc("GET /api/v1/auth/preferences", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleGetPreferences))
	mux.HandleFunc("PUT /api/v1/auth/preferences", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handlePutPreferences))
	mux.HandleFunc("POST /api/v1/auth/export", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleStartExport))
	mux.HandleFunc("GET /api/v1/auth/export/{id}", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleExportStatus))
	mux.HandleFunc("GET /api/v1/auth/export/{id}/download", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleDownloadExport))
	mux.HandleFunc("GET /api/v1/auth/delete-account/check", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleDeleteAccountCheck))
	mux.HandleFunc("DELETE /api/v1/auth/account", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleDeleteAccount))
	mux.HandleFunc("DELETE /api/v1/auth/link/{provider}", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleUnlinkProvider))
	mux.HandleFunc("GET /api/v1/auth/email-change", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleEmailChangeStatus))
	mux.HandleFunc("POST /api/v1/auth/email-change", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleRequestEmailChange))
	mux.HandleFunc("POST /api/v1/auth/email-change/confirm", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleConfirmEmailChange))
}
