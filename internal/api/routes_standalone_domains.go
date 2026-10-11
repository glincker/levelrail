package api

import "net/http"

// Standalone domains proxy public traffic to any port or address this server
// can reach, so every write is AbilityWriteSensitive like firewall rules. All
// routes are instance wide: members of only non-default teams get 403.
func (rt *Router) registerStandaloneDomainRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/domains/standalone", rt.requireAbility(AbilityRead, rt.handleListStandaloneDomains))
	mux.HandleFunc("POST /api/v1/domains/standalone", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateStandaloneDomain))
	mux.HandleFunc("POST /api/v1/domains/standalone/validate", rt.requireAbility(AbilityWriteSensitive, rt.handleValidateStandaloneDomain))
	mux.HandleFunc("GET /api/v1/domains/standalone/candidates", rt.requireAbility(AbilityWriteSensitive, rt.handleStandaloneCandidates))
	mux.HandleFunc("PUT /api/v1/domains/standalone/{domain}", rt.requireAbility(AbilityWriteSensitive, rt.handleUpdateStandaloneDomain))
	mux.HandleFunc("DELETE /api/v1/domains/standalone/{domain}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteStandaloneDomain))
	// The address DNS records must point at. Reading is AbilityRead; the
	// manual override changes DNS checks for every domain, so it is root.
	mux.HandleFunc("GET /api/v1/system/server-address", rt.requireAbility(AbilityRead, rt.handleGetServerAddress))
	mux.HandleFunc("PUT /api/v1/system/server-address", rt.requireAbility(AbilityRoot, rt.handleSetServerAddress))
}
