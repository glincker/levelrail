package api

import "net/http"

// registerMailDNSRoutes wires the experimental mail DNS helper. Plan is a
// read-gated dry run; only apply writes to the provider and needs root.
func (rt *Router) registerMailDNSRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/mail/dns/plan", rt.requireAbility(AbilityRead, rt.handleMailDNSPlan))
	mux.HandleFunc("POST /api/v1/mail/dns/apply", rt.requireAbility(AbilityRoot, rt.handleMailDNSApply))
	mux.HandleFunc("GET /api/v1/mail/status", rt.requireAbility(AbilityRead, rt.handleMailStatus))
	mux.HandleFunc("GET /api/v1/mail/dns/check", rt.requireAbility(AbilityRead, rt.handleMailDNSCheck))
	mux.HandleFunc("GET /api/v1/mail/dns/provider", rt.requireAbility(AbilityRead, rt.handleMailDNSProvider))
	mux.HandleFunc("GET /api/v1/mail/dns/domains", rt.requireAbility(AbilityRead, rt.handleMailDNSDomains))
	mux.HandleFunc("GET /api/v1/mail/dns/applied", rt.requireAbility(AbilityRead, rt.handleMailDNSApplied))
	// Not under /mail/, so the mail flag does not gate it: the Domains page uses it too.
	mux.HandleFunc("GET /api/v1/domains/dns-provider", rt.requireAbility(AbilityRead, rt.handleDomainDNSProvider))
	// Always registered so route guards see it; the handler answers 404 with demo off.
	mux.HandleFunc("POST /api/v1/mail/demo/reset", rt.requireAbility(AbilityRoot, rt.handleMailDemoReset))
}
