package api

import "net/http"

// Managed proxy routes write into a foreign proxy's directory and switch TLS
// handling for every domain, so every write is AbilityRoot.
func (rt *Router) registerProxyIntegrationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/system/proxy-integration", rt.requireAbility(AbilityRead, rt.handleGetProxyIntegration))
	mux.HandleFunc("POST /api/v1/system/proxy-integration/setup", rt.requireAbility(AbilityRoot, rt.handleSetupProxyIntegration))
	mux.HandleFunc("POST /api/v1/system/proxy-integration/apply", rt.requireAbility(AbilityRoot, rt.handleApplyProxyIntegration))
	mux.HandleFunc("POST /api/v1/system/proxy-integration/verify", rt.requireAbility(AbilityRoot, rt.handleVerifyProxyIntegration))
	mux.HandleFunc("PUT /api/v1/settings/proxy-integration", rt.requireAbility(AbilityRoot, rt.handleUpdateProxyIntegrationSettings))
}
