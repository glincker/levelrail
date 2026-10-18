package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// registerDomainPolicyRoutes wires the per-domain traffic controls
// (domain_policies*.go). Literal registrations so gen-api-reference sees them.
func (rt *Router) registerDomainPolicyRoutes(mux *http.ServeMux) {
	// Per-domain header rules and presets. Reads are AbilityRead; changes are
	// AbilityDeploy like the other per-domain routing settings.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/headers", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDomainPolicy(trafficpolicy.KindHeaders)))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/headers", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetDomainPolicy(trafficpolicy.KindHeaders)))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/headers", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeleteDomainPolicy(trafficpolicy.KindHeaders)))
	// Ordered path forwarders to another app, an external URL, or a redirect.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/forwarders", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDomainPolicy(trafficpolicy.KindForwarders)))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/forwarders", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetDomainPolicy(trafficpolicy.KindForwarders)))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/forwarders", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeleteDomainPolicy(trafficpolicy.KindForwarders)))
	// Country allow or deny list.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/geo", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDomainPolicy(trafficpolicy.KindGeo)))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/geo", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetDomainPolicy(trafficpolicy.KindGeo)))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/geo", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeleteDomainPolicy(trafficpolicy.KindGeo)))
	// Per-client rate, concurrency, body size and bad bot limits, with an
	// emergency under-attack switch that expires on its own.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/shield", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDomainPolicy(trafficpolicy.KindShield)))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/shield", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetDomainPolicy(trafficpolicy.KindShield)))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/shield", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeleteDomainPolicy(trafficpolicy.KindShield)))
	mux.HandleFunc("POST /api/v1/apps/{name}/domains/{domain}/shield/under-attack", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleStartUnderAttack))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/shield/under-attack", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleStopUnderAttack))
	mux.HandleFunc("GET /api/v1/system/shield/bots", rt.requireAbility(AbilityRead, rt.handleShieldBotList))
	// Ingress response cache rules, purge and per-domain stats.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/cache", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDomainPolicy(trafficpolicy.KindCache)))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/cache", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetDomainPolicy(trafficpolicy.KindCache)))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/cache", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeleteDomainPolicy(trafficpolicy.KindCache)))
	mux.HandleFunc("POST /api/v1/apps/{name}/domains/{domain}/cache/purge", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handlePurgeDomainCache))
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/cache/stats", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDomainCacheStats))
	// App-level cache view, purge across the app's domains, purge history
	// and the one-click static assets preset; instance cache and CDN overview.
	mux.HandleFunc("GET /api/v1/apps/{name}/cache", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppCache))
	mux.HandleFunc("POST /api/v1/apps/{name}/cache/purge", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handlePurgeAppCache))
	mux.HandleFunc("GET /api/v1/apps/{name}/cache/purges", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListCachePurges))
	mux.HandleFunc("POST /api/v1/apps/{name}/cache/preset", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleApplyCachePreset))
	mux.HandleFunc("GET /api/v1/system/cache", rt.requireAbility(AbilityRead, rt.handleSystemCache))
	mux.HandleFunc("GET /api/v1/cdn/overview", rt.requireAbility(AbilityRead, rt.handleCDNOverview))
	// Every section plus status for the domain page, and a dry run preview.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/policies", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDomainPolicies))
	mux.HandleFunc("POST /api/v1/apps/{name}/domains/{domain}/policies/preview", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handlePreviewDomainPolicies))
	// Force HTTPS, trailing slash, lower case host, www and apex, aliases.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/redirects", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDomainRedirects))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/redirects", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetDomainRedirectSettings))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/redirects", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeleteDomainRedirectSettings))
	mux.HandleFunc("POST /api/v1/apps/{name}/domains/{domain}/redirects/canonical", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetDomainCanonical))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/redirects/aliases", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetDomainAliases))
	// The app's raw streams; restricting a port writes firewall rules, so it
	// needs AbilityWriteSensitive like the firewall routes.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/ports", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDomainPorts))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/ports/{port}/restrict", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleRestrictDomainPort))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/ports/{port}/restrict", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleUnrestrictDomainPort))
	// Active country sources and a test lookup for one address.
	mux.HandleFunc("GET /api/v1/system/geoip", rt.requireAbility(AbilityRead, rt.handleGeoIPLookup))
}
