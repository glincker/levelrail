package api

import "net/http"

// registerIngressRoutes registers the certificates, ingress settings, gate and domains routes.
func (rt *Router) registerIngressRoutes(mux *http.ServeMux) {
	// Certificates (TLS renewal visibility): this project treats
	// "a cert renewal fails silently at 3am" as its central
	// risk to catch before it bites a real user, and until now nothing
	// in the API surfaced certificate state at all. Read-only, so
	// AbilityRead like handleSystemStatus/handleGetNodeHealth, not
	// AbilityRoot: seeing whether a domain's certificate is about to
	// expire is ordinary operator visibility, not a fleet-admin
	// mutation the way the node routes above are.
	mux.HandleFunc("GET /api/v1/certificates", rt.requireAbility(AbilityRead, rt.handleListCertificates))

	// Ingress settings: GET is AbilityRead. PUT is AbilityRoot, since flipping acme_enabled changes
	// certificate automation fleet-wide on the next reconcile.
	mux.HandleFunc("GET /api/v1/settings/ingress", rt.requireAbility(AbilityRead, rt.handleGetIngressSettings))
	mux.HandleFunc("PUT /api/v1/settings/ingress", rt.requireAbility(AbilityRoot, rt.handleUpdateIngressSettings))
	mux.HandleFunc("GET /api/v1/settings/dashboard-url", rt.requireAbility(AbilityRead, rt.handleGetDashboardURL))
	mux.HandleFunc("PUT /api/v1/settings/dashboard-url", rt.requireAbility(AbilityRoot, rt.handleUpdateDashboardURL))
	mux.HandleFunc("GET /api/v1/settings/heartbeat", rt.requireAbility(AbilityRoot, rt.handleGetHeartbeat))
	mux.HandleFunc("PUT /api/v1/settings/heartbeat", rt.requireAbility(AbilityRoot, rt.handlePutHeartbeat))

	// Platform-level DNS check (ingress_settings.go): runDomainCheck
	// against the platform's own PrimaryDomain instead of a per-app one.
	// AbilityRoot, matching PUT /api/v1/settings/ingress just above: the
	// result speaks directly to whether that endpoint's ACMEEnabled
	// toggle can actually succeed.
	mux.HandleFunc("GET /api/v1/settings/ingress/https", rt.requireAbility(AbilityRead, rt.handleGetHTTPSStatus))
	mux.HandleFunc("POST /api/v1/settings/ingress/https", rt.requireAbility(AbilityRoot, rt.handleEnableHTTPS))
	mux.HandleFunc("GET /api/v1/settings/ingress/check", rt.requireAbility(AbilityRoot, rt.handleCheckIngressDomain))

	mux.HandleFunc("GET /api/v1/settings/dns-provider", rt.requireAbility(AbilityRead, rt.handleGetDNSProvider))
	mux.HandleFunc("PUT /api/v1/settings/dns-provider", rt.requireAbility(AbilityRoot, rt.handleSetDNSProvider))
	mux.HandleFunc("POST /api/v1/settings/dns-provider/test", rt.requireAbility(AbilityRoot, rt.handleTestDNSProvider))
	mux.HandleFunc("GET /api/v1/apps/{name}/dns-provider", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppDNSProvider))
	mux.HandleFunc("PUT /api/v1/apps/{name}/dns-provider", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetAppDNSProvider))
	mux.HandleFunc("POST /api/v1/apps/{name}/dns-provider/test", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleTestAppDNSProvider))

	// Domain DNS check (domain_check.go): the guidance layer on top of
	// domain connection, so DomainEditor can show an operator the exact
	// DNS record to add and watch it flip to "connected" once it actually
	// resolves. AbilityRead, same passive-visibility tier as GET
	// /apps/{name}/git-source: a live DNS lookup, no write.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/check", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleCheckDomain))

	// Domain basic auth: GET is AbilityRead. PUT/DELETE are AbilityRoot, since they change how a
	// live host is secured.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/auth", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDomainBasicAuth))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/auth", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleSetDomainBasicAuth))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/auth", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleClearDomainBasicAuth))
	// Auth gate: same tiers as basic auth, it decides who reaches the host.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/auth-gate", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDomainAuthGate))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/auth-gate", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleSetDomainAuthGate))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/auth-gate", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleClearDomainAuthGate))
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/auth-gate/events", rt.requireAbilityForResource(AbilityReadSensitive, appResourceFromPath, rt.handleListAuthGateEvents))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/auth-gate/sessions", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleRevokeAuthGateSessions))
	// Unauthenticated by design: the check proves itself with the ingress's
	// gate secret, and login requires a dashboard session before minting.
	mux.HandleFunc("GET /api/v1/gate/check", rt.handleGateCheck)
	mux.HandleFunc("GET /gate/login", rt.handleGateLogin)

	// Maintenance mode: GET is AbilityRead, matching the auth routes'
	// own passive-visibility tier. PUT/DELETE are AbilityDeploy, not
	// AbilityRoot: this changes an app's runtime routing behavior, the
	// same "app lifecycle" tier POST .../stop and .../start already
	// use, not a credential-bearing change like basic auth.
	mux.HandleFunc("GET /api/v1/apps/{name}/maintenance", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppMaintenance))
	mux.HandleFunc("PUT /api/v1/apps/{name}/maintenance", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetAppMaintenance))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/maintenance", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleClearAppMaintenance))
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/maintenance", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDomainMaintenance))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/maintenance", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetDomainMaintenance))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/maintenance", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleClearDomainMaintenance))

	// BYO TLS certificate upload (domain_tls_cert.go): an operator-
	// supplied certificate/key pair used in place of Caddy's automatic
	// ACME/internal issuance for one app-owned domain, enforced on the
	// next ingress reconcile pass. GET is AbilityRead, matching the auth
	// routes' own passive-visibility tier. PUT/DELETE are AbilityRoot,
	// the same "real infrastructure, high blast radius" tier PUT/DELETE
	// .../domains/{domain}/auth already reserves for a credential-bearing
	// change.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/tls-cert", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDomainTLSCert))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/tls-cert", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleSetDomainTLSCert))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/tls-cert", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleClearDomainTLSCert))

	// Force re-issuance of domain's automatic certificate (domain_tls_cert.go's
	// handleRenewDomainCertificate): deletes the stored certmagic entry
	// and nudges the reconciler. AbilityRoot, same tier as PUT/DELETE
	// .../tls-cert above: real infra, real blast radius.
	mux.HandleFunc("POST /api/v1/apps/{name}/domains/{domain}/cert/renew", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleRenewDomainCertificate))

	// Opt-in WAF and rate limiting (domain_waf.go): OWASP Coraza and
	// Caddy's rate_limit handler on one app-owned domain, enforced on
	// the next ingress reconcile pass. GET is AbilityRead, matching the
	// auth/maintenance routes' own passive-visibility tier. PUT/DELETE
	// are AbilityDeploy, the same "app lifecycle, runtime routing
	// behavior, not a credential" tier PUT/DELETE .../maintenance
	// already uses: unlike basic auth or a BYO cert, nothing here is
	// secret material.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/waf", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDomainWAF))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/waf", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetDomainWAF))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/waf", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleClearDomainWAF))

	// DNS records (dns_records.go): list/add/edit/delete the actual
	// A/AAAA/CNAME/TXT/MX/SRV/CAA records in one app-owned domain's
	// best-effort zone, via whichever ACME DNS-01 provider (Cloudflare or
	// Route53) is configured. GET is AbilityRead: a live provider API
	// read plus a DNS lookup, no write. POST/PUT/DELETE are AbilityRoot,
	// the same "real infrastructure, high blast radius" tier PUT/DELETE
	// .../tls-cert and .../auth already reserve: this writes directly to
	// a live, externally visible DNS zone.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/dns-records", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListDNSRecords))
	mux.HandleFunc("POST /api/v1/apps/{name}/domains/{domain}/dns-records", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleCreateDNSRecord))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/dns-records", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleUpdateDNSRecord))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/dns-records", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleDeleteDNSRecord))
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/go-live", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGoLiveStatus))
	mux.HandleFunc("POST /api/v1/apps/{name}/domains/{domain}/go-live", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleGoLiveRun))
	mux.HandleFunc("POST /api/v1/apps/{name}/domains/go-live/plan", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleGoLivePlan))
	mux.HandleFunc("GET /api/v1/settings/domain-automation", rt.requireAbility(AbilityRead, rt.handleGetDomainAutomation))
	mux.HandleFunc("PUT /api/v1/settings/domain-automation", rt.requireAbility(AbilityRoot, rt.handleUpdateDomainAutomation))
	mux.HandleFunc("GET /api/v1/domains/automation/runs", rt.requireAbility(AbilityRead, rt.handleListAutomationRuns))
	mux.HandleFunc("POST /api/v1/domains/automation/runs/{id}/undo", rt.requireAbility(AbilityRoot, rt.handleUndoAutomationRun))
	mux.HandleFunc("GET /api/v1/dns/zone", rt.requireAbility(AbilityRead, rt.handleDNSZone))
	mux.HandleFunc("POST /api/v1/settings/ingress/apps-base-domain/backfill", rt.requireAbility(AbilityRoot, rt.handleBackfillBaseDomain))

	// Domain redirect (domain_redirect.go): points one app-owned domain
	// at an arbitrary target URL, enforced by Caddy's static_response
	// handler on the next ingress reconcile pass. GET is AbilityRead,
	// matching the auth/maintenance/waf routes' own passive-visibility
	// tier. PUT/DELETE are AbilityDeploy, the same "app lifecycle,
	// runtime routing behavior, not a credential" tier PUT/DELETE
	// .../maintenance and .../waf already use.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/redirect", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDomainRedirect))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/redirect", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetDomainRedirect))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/redirect", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleClearDomainRedirect))

	// Custom error pages (domain_error_pages.go): replaces Caddy's bare
	// default error text, or whatever the backend itself returned, with
	// the operator's own HTML for a fixed set of status codes (404, 500,
	// 502, 503), enforced on the next ingress reconcile pass. GET is
	// AbilityRead; PUT/DELETE are AbilityDeploy, the same tier
	// PUT/DELETE .../waf already uses.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/error-pages", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDomainErrorPages))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/error-pages", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetDomainErrorPage))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/error-pages", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleClearDomainErrorPages))
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/search-visibility", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDomainSearchVisibility))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/search-visibility", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetDomainSearchVisibility))
	rt.registerDomainPolicyRoutes(mux)
	rt.registerFirewallEdgeRoutes(mux)
}
