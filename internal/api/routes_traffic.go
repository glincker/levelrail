package api

import "net/http"

// registerTrafficRoutes wires the domains, DNS and proxy health surface.
func (rt *Router) registerTrafficRoutes(mux *http.ServeMux) {
	// Counts for the sidebar attention badge and the domains health strip,
	// from stored state and the last known DNS checks only.
	mux.HandleFunc("GET /api/v1/traffic/summary", rt.requireAbility(AbilityRead, rt.handleTrafficSummary))
	// Domain doctor: ordered DNS, CAA, port, TLS, HTTP, redirect and HSTS
	// checks with one fix each. Read tier: it only reads and probes, and it
	// connects only when the domain resolves to this server.
	mux.HandleFunc("POST /api/v1/apps/{name}/domains/{domain}/doctor", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleDomainDoctor))
	// One domain's activity timeline: audit rows plus certificate failures,
	// cursor paginated by ?before. Visible to callers who can see its app.
	mux.HandleFunc("GET /api/v1/domains/{domain}/activity", rt.requireAbility(AbilityRead, rt.handleDomainActivity))
}
