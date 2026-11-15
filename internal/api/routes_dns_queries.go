package api

import "net/http"

func (rt *Router) registerDNSQueryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/dns/queries", rt.requireAbility(AbilityRead, rt.handleListDNSQueries))
	mux.HandleFunc("GET /api/v1/dns/stats", rt.requireAbility(AbilityRead, rt.handleDNSStats))
}
