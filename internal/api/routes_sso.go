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
	mux.HandleFunc("GET /api/v1/sso/saml/connections/{id}/test/start", rt.requireAbility(AbilityRoot, rt.handleStartSAMLTest))
	mux.HandleFunc("GET /api/v1/sso/saml/connections/{id}/test/result", rt.requireAbility(AbilityRoot, rt.handleSAMLTestResult))
	mux.HandleFunc("GET /api/v1/sso/oidc", rt.requireAbility(AbilityRoot, rt.handleListOAuthSSOPolicies))
	mux.HandleFunc("GET /api/v1/sso/oidc/{provider}", rt.requireAbility(AbilityRoot, rt.handleGetOAuthSSOPolicy))
	mux.HandleFunc("PUT /api/v1/sso/oidc/{provider}", rt.requireAbility(AbilityRoot, rt.handlePutOAuthSSOPolicy))
	mux.HandleFunc("POST /api/v1/sso/oidc/{provider}/evaluate", rt.requireAbility(AbilityRoot, rt.handleEvaluateOAuthSSO))
	mux.HandleFunc("GET /api/v1/sso/oidc/{provider}/test/start", rt.requireAbility(AbilityRoot, rt.handleStartOAuthSSOTest))
	mux.HandleFunc("GET /api/v1/sso/oidc/{provider}/test/result", rt.requireAbility(AbilityRoot, rt.handleOAuthSSOTestResult))
	mux.HandleFunc("GET /api/v1/sso/group-mappings", rt.requireAbility(AbilityRoot, rt.handleListSSOGroupMappings))
	mux.HandleFunc("PUT /api/v1/sso/group-mappings", rt.requireAbility(AbilityRoot, rt.handlePutSSOGroupMappings))
	mux.HandleFunc("GET /api/v1/sso/settings", rt.requireAbility(AbilityRoot, rt.handleGetSSOSettings))
	mux.HandleFunc("PUT /api/v1/sso/settings", rt.requireAbility(AbilityRoot, rt.handlePutSSOSettings))
	mux.HandleFunc("GET /api/v1/sso/scim/tokens", rt.requireAbility(AbilityRoot, rt.handleListSCIMTokens))
	mux.HandleFunc("POST /api/v1/sso/scim/tokens", rt.requireAbility(AbilityRoot, rt.handleCreateSCIMToken))
	mux.HandleFunc("POST /api/v1/sso/scim/tokens/{id}/rotate", rt.requireAbility(AbilityRoot, rt.handleRotateSCIMToken))
	mux.HandleFunc("DELETE /api/v1/sso/scim/tokens/{id}", rt.requireAbility(AbilityRoot, rt.handleRevokeSCIMToken))

	mux.HandleFunc("GET /api/v1/security/session-policies", rt.requireAbility(AbilityRoot, rt.handleListSessionPolicies))
	mux.HandleFunc("PUT /api/v1/security/session-policies/{team_id}", rt.requireAbility(AbilityRoot, rt.handlePutSessionPolicy))
	mux.HandleFunc("DELETE /api/v1/security/session-policies/{team_id}", rt.requireAbility(AbilityRoot, rt.handleDeleteSessionPolicy))
	mux.HandleFunc("GET /api/v1/security/session-policy/effective", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleEffectiveSessionPolicy))

	// SCIM 2.0, served by the auth library behind handleSCIM: authenticated by a
	// SCIM bearer token, never by a session.
	mux.HandleFunc("GET /api/v1/scim/v2/ServiceProviderConfig", rt.handleSCIM)
	mux.HandleFunc("GET /api/v1/scim/v2/ResourceTypes", rt.handleSCIM)
	mux.HandleFunc("GET /api/v1/scim/v2/ResourceTypes/{id}", rt.handleSCIM)
	mux.HandleFunc("GET /api/v1/scim/v2/Schemas", rt.handleSCIM)
	mux.HandleFunc("GET /api/v1/scim/v2/Schemas/{id}", rt.handleSCIM)
	mux.HandleFunc("GET /api/v1/scim/v2/Users", rt.handleSCIM)
	mux.HandleFunc("POST /api/v1/scim/v2/Users", rt.handleSCIM)
	mux.HandleFunc("GET /api/v1/scim/v2/Users/{id}", rt.handleSCIM)
	mux.HandleFunc("PUT /api/v1/scim/v2/Users/{id}", rt.handleSCIM)
	mux.HandleFunc("PATCH /api/v1/scim/v2/Users/{id}", rt.handleSCIM)
	mux.HandleFunc("DELETE /api/v1/scim/v2/Users/{id}", rt.handleSCIM)
	mux.HandleFunc("GET /api/v1/scim/v2/Groups", rt.handleSCIM)
	mux.HandleFunc("POST /api/v1/scim/v2/Groups", rt.handleSCIM)
	mux.HandleFunc("GET /api/v1/scim/v2/Groups/{id}", rt.handleSCIM)
	mux.HandleFunc("PUT /api/v1/scim/v2/Groups/{id}", rt.handleSCIM)
	mux.HandleFunc("PATCH /api/v1/scim/v2/Groups/{id}", rt.handleSCIM)
	mux.HandleFunc("DELETE /api/v1/scim/v2/Groups/{id}", rt.handleSCIM)
}
