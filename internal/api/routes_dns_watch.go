package api

import "net/http"

func (rt *Router) registerDNSWatchRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/domains/watch", rt.requireAbility(AbilityRead, rt.handleDomainsWatch))
	mux.HandleFunc("GET /api/v1/domains/{domain}/watch", rt.requireAbility(AbilityRead, rt.handleDomainWatchSnapshot))
}
