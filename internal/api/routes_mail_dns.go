package api

import "net/http"

// registerMailDNSRoutes wires the experimental mail DNS helper. Plan is a
// read-gated dry run; only apply writes to the provider and needs root.
func (rt *Router) registerMailDNSRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/mail/dns/plan", rt.requireAbility(AbilityRead, rt.handleMailDNSPlan))
	mux.HandleFunc("POST /api/v1/mail/dns/apply", rt.requireAbility(AbilityRoot, rt.handleMailDNSApply))
	mux.HandleFunc("GET /api/v1/mail/dns/check", rt.requireAbility(AbilityRead, rt.handleMailDNSCheck))
}
