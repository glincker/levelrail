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
	mux.HandleFunc("GET /api/v1/sso/scim/tokens", rt.requireAbility(AbilityRoot, rt.handleListSCIMTokens))
	mux.HandleFunc("POST /api/v1/sso/scim/tokens", rt.requireAbility(AbilityRoot, rt.handleCreateSCIMToken))
	mux.HandleFunc("POST /api/v1/sso/scim/tokens/{id}/rotate", rt.requireAbility(AbilityRoot, rt.handleRotateSCIMToken))
	mux.HandleFunc("DELETE /api/v1/sso/scim/tokens/{id}", rt.requireAbility(AbilityRoot, rt.handleRevokeSCIMToken))

	mux.HandleFunc("GET /api/v1/security/session-policies", rt.requireAbility(AbilityRoot, rt.handleListSessionPolicies))
	mux.HandleFunc("PUT /api/v1/security/session-policies/{team_id}", rt.requireAbility(AbilityRoot, rt.handlePutSessionPolicy))
	mux.HandleFunc("DELETE /api/v1/security/session-policies/{team_id}", rt.requireAbility(AbilityRoot, rt.handleDeleteSessionPolicy))
	mux.HandleFunc("GET /api/v1/security/session-policy/effective", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleEffectiveSessionPolicy))

	// SCIM 2.0: authenticated by a SCIM bearer token in requireSCIM, never by a session.
	mux.HandleFunc("GET /api/v1/scim/v2/ServiceProviderConfig", rt.requireSCIM(rt.handleSCIMServiceProviderConfig))
	mux.HandleFunc("GET /api/v1/scim/v2/ResourceTypes", rt.requireSCIM(rt.handleSCIMResourceTypes))
	mux.HandleFunc("GET /api/v1/scim/v2/Users", rt.requireSCIM(rt.handleSCIMListUsers))
	mux.HandleFunc("POST /api/v1/scim/v2/Users", rt.requireSCIM(rt.handleSCIMCreateUser))
	mux.HandleFunc("GET /api/v1/scim/v2/Users/{id}", rt.requireSCIM(rt.handleSCIMGetUser))
	mux.HandleFunc("PUT /api/v1/scim/v2/Users/{id}", rt.requireSCIM(rt.handleSCIMReplaceUser))
	mux.HandleFunc("PATCH /api/v1/scim/v2/Users/{id}", rt.requireSCIM(rt.handleSCIMPatchUser))
	mux.HandleFunc("DELETE /api/v1/scim/v2/Users/{id}", rt.requireSCIM(rt.handleSCIMDeleteUser))
	mux.HandleFunc("GET /api/v1/scim/v2/Groups", rt.requireSCIM(rt.handleSCIMListGroups))
	mux.HandleFunc("POST /api/v1/scim/v2/Groups", rt.requireSCIM(rt.handleSCIMCreateGroup))
	mux.HandleFunc("GET /api/v1/scim/v2/Groups/{id}", rt.requireSCIM(rt.handleSCIMGetGroup))
	mux.HandleFunc("PUT /api/v1/scim/v2/Groups/{id}", rt.requireSCIM(rt.handleSCIMReplaceGroup))
	mux.HandleFunc("PATCH /api/v1/scim/v2/Groups/{id}", rt.requireSCIM(rt.handleSCIMPatchGroup))
	mux.HandleFunc("DELETE /api/v1/scim/v2/Groups/{id}", rt.requireSCIM(rt.handleSCIMDeleteGroup))
}
