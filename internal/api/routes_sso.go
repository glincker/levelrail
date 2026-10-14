package api

import "net/http"

// registerSSORoutes wires SAML sign-in and its root-only administration. The
// /auth/saml routes are public by nature: the IdP and the browser reach them
// before any session exists, and each one authenticates by its own means.
func (rt *Router) registerSSORoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/auth/saml/providers", rt.handleListPublicSAMLProviders)
	mux.HandleFunc("GET /api/v1/auth/saml/{id}/metadata", rt.handleSAMLMetadata)
	mux.HandleFunc("GET /api/v1/auth/saml/{id}/login", rt.handleSAMLLogin)
	mux.HandleFunc("POST /api/v1/auth/saml/{id}/acs", rt.handleSAMLACS)

	mux.HandleFunc("GET /api/v1/sso/saml/connections", rt.requireAbility(AbilityRoot, rt.handleListSAMLConnections))
	mux.HandleFunc("POST /api/v1/sso/saml/connections", rt.requireAbility(AbilityRoot, rt.handleCreateSAMLConnection))
	mux.HandleFunc("GET /api/v1/sso/saml/connections/{id}", rt.requireAbility(AbilityRoot, rt.handleGetSAMLConnection))
	mux.HandleFunc("PUT /api/v1/sso/saml/connections/{id}", rt.requireAbility(AbilityRoot, rt.handleUpdateSAMLConnection))
	mux.HandleFunc("DELETE /api/v1/sso/saml/connections/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteSAMLConnection))
	mux.HandleFunc("POST /api/v1/sso/saml/connections/{id}/metadata", rt.requireAbility(AbilityRoot, rt.handleImportSAMLMetadata))
	mux.HandleFunc("GET /api/v1/sso/group-mappings", rt.requireAbility(AbilityRoot, rt.handleListSSOGroupMappings))
	mux.HandleFunc("PUT /api/v1/sso/group-mappings", rt.requireAbility(AbilityRoot, rt.handlePutSSOGroupMappings))
	mux.HandleFunc("GET /api/v1/sso/settings", rt.requireAbility(AbilityRoot, rt.handleGetSSOSettings))
	mux.HandleFunc("PUT /api/v1/sso/settings", rt.requireAbility(AbilityRoot, rt.handlePutSSOSettings))
}
