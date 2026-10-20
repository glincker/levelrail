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
	// Guided public certificate: preflight, request, progress. Request is root tier.
	mux.HandleFunc("GET /api/v1/domains/{domain}/cert/preflight", rt.requireAbility(AbilityRead, rt.handleCertPreflight))
	mux.HandleFunc("GET /api/v1/domains/{domain}/cert/status", rt.requireAbility(AbilityRead, rt.handleCertStatus))
	mux.HandleFunc("POST /api/v1/domains/{domain}/cert", rt.requireAbility(AbilityRoot, rt.handleGetCert))
	// Same doctor addressed by domain alone, as one JSON report or a
	// stream of checks; the owning app is looked up and visibility enforced.
	mux.HandleFunc("POST /api/v1/domains/{domain}/diagnose", rt.requireAbility(AbilityRead, rt.handleDomainDiagnose))
	mux.HandleFunc("GET /api/v1/domains/{domain}/diagnose/stream", rt.requireAbility(AbilityRead, rt.withStreamReauth(domainDiagnoseResource, rt.handleDomainDiagnoseStream)))
	// One domain's activity timeline: audit rows plus certificate failures,
	// cursor paginated by ?before. Visible to callers who can see its app.
	mux.HandleFunc("GET /api/v1/domains/{domain}/activity", rt.requireAbility(AbilityRead, rt.handleDomainActivity))
	// Go live checklist: app, port, firewall, DNS, certificate and HTTPS in one
	// ordered report, a stream that refreshes it, and stored-state badges.
	// Fixes apply through the existing root routes (fixes, cert, dns-records).
	mux.HandleFunc("GET /api/v1/apps/{name}/go-live/checklist", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGoLiveChecklist))
	mux.HandleFunc("GET /api/v1/apps/{name}/go-live/stream", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.withStreamReauth(appResourceFromPath, rt.handleGoLiveChecklistStream)))
	mux.HandleFunc("GET /api/v1/go-live/status", rt.requireAbility(AbilityRead, rt.handleLiveBadges))
}
