package api

import "net/http"

// registerStatusReasonRoutes wires the read-only status reason catalog.
func (rt *Router) registerStatusReasonRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/status-reasons", rt.requireAbility(AbilityRead, rt.handleListStatusReasons))
}
