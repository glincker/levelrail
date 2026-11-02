package api

import "net/http"

// Parked domains stay registered with their DNS and certificate but are routed
// by no app. Lifecycle changes are instance wide like standalone domains, so
// every write is AbilityWriteSensitive and tenant-only members get 403.
func (rt *Router) registerParkedDomainRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/domains/parked", rt.requireAbility(AbilityRead, rt.handleListParkedDomains))
	mux.HandleFunc("GET /api/v1/domains/parked/retention", rt.requireAbility(AbilityRead, rt.handleParkedRetention))
	mux.HandleFunc("POST /api/v1/domains/park", rt.requireAbility(AbilityWriteSensitive, rt.handleParkDomain))
	mux.HandleFunc("POST /api/v1/domains/batch", rt.requireAbility(AbilityWriteSensitive, rt.handleDomainsBatch))
	mux.HandleFunc("POST /api/v1/domains/move", rt.requireAbility(AbilityWriteSensitive, rt.handleMoveDomain))
	mux.HandleFunc("POST /api/v1/domains/parked/{domain}/attach", rt.requireAbility(AbilityWriteSensitive, rt.handleAttachParkedDomain))
	mux.HandleFunc("DELETE /api/v1/domains/parked/{domain}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteParkedDomain))
}
