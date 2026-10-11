package api

import "net/http"

// registerSecurityRoutes wires the security center: posture, policy,
// sessions and devices, the account switch and the "this wasn't me" link.
func (rt *Router) registerSecurityRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/security/posture", rt.requireAbility(AbilityRead, rt.handleSecurityPosture))
	mux.HandleFunc("GET /api/v1/security/policy", rt.requireAbility(AbilityRead, rt.handleGetSecurityPolicy))
	mux.HandleFunc("PUT /api/v1/security/policy", rt.requireAbility(AbilityRoot, rt.handlePutSecurityPolicy))
	mux.HandleFunc("GET /api/v1/security/sessions", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleListSecuritySessions))
	mux.HandleFunc("DELETE /api/v1/security/sessions/{id}", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleRevokeSecuritySession))
	mux.HandleFunc("POST /api/v1/security/sessions/revoke-others", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleRevokeOtherSecuritySessions))
	mux.HandleFunc("GET /api/v1/security/account", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleGetAccountSecurity))
	mux.HandleFunc("PUT /api/v1/security/account", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handlePutAccountSecurity))
	mux.HandleFunc("POST /api/v1/auth/sign-in-alert/disown", rt.handleDisownSignIn)
}
