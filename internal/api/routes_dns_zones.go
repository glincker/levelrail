package api

import "net/http"

// registerDNSZoneRoutes wires zone level DNS management. Reads need read, every
// write against the provider's live zone needs root.
func (rt *Router) registerDNSZoneRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/dns/zones", rt.requireAbility(AbilityRead, rt.handleListDNSZones))
	mux.HandleFunc("POST /api/v1/dns/zones", rt.requireAbility(AbilityRoot, rt.handleCreateDNSZone))
	mux.HandleFunc("GET /api/v1/dns/zones/{zone}", rt.requireAbility(AbilityRead, rt.handleGetDNSZone))
	mux.HandleFunc("DELETE /api/v1/dns/zones/{zone}", rt.requireAbility(AbilityRoot, rt.handleDeleteDNSZone))
	mux.HandleFunc("GET /api/v1/dns/zones/{zone}/nameservers", rt.requireAbility(AbilityRead, rt.handleDNSZoneNameServers))
	mux.HandleFunc("GET /api/v1/dns/zones/{zone}/delegation", rt.requireAbility(AbilityRead, rt.handleDNSZoneDelegation))
	mux.HandleFunc("GET /api/v1/dns/zones/{zone}/discover", rt.requireAbility(AbilityRead, rt.handleDiscoverDNSZoneRecords))
	mux.HandleFunc("GET /api/v1/dns/zones/{zone}/records", rt.requireAbility(AbilityRead, rt.handleListDNSZoneRecords))
	mux.HandleFunc("POST /api/v1/dns/zones/{zone}/records", rt.requireAbility(AbilityRoot, rt.handleCreateDNSZoneRecord))
	mux.HandleFunc("PUT /api/v1/dns/zones/{zone}/records", rt.requireAbility(AbilityRoot, rt.handleUpdateDNSZoneRecord))
	mux.HandleFunc("DELETE /api/v1/dns/zones/{zone}/records", rt.requireAbility(AbilityRoot, rt.handleDeleteDNSZoneRecord))
	mux.HandleFunc("POST /api/v1/dns/zones/{zone}/records/import", rt.requireAbility(AbilityRoot, rt.handleImportDNSZoneRecords))
	mux.HandleFunc("GET /api/v1/dns/zones/{zone}/records/export", rt.requireAbility(AbilityRead, rt.handleExportDNSZoneRecords))
	mux.HandleFunc("POST /api/v1/dns/zones/{zone}/templates/{id}", rt.requireAbility(AbilityRoot, rt.handleApplyDNSTemplate))
	mux.HandleFunc("GET /api/v1/dns/templates", rt.requireAbility(AbilityRead, rt.handleListDNSTemplates))
	mux.HandleFunc("GET /api/v1/dns/check", rt.requireAbility(AbilityRead, rt.handleDNSCheck))
	mux.HandleFunc("GET /api/v1/dns/health-checks", rt.requireAbility(AbilityRead, rt.handleListDNSHealthChecks))
	mux.HandleFunc("POST /api/v1/dns/health-checks", rt.requireAbility(AbilityRoot, rt.handleCreateDNSHealthCheck))
	mux.HandleFunc("DELETE /api/v1/dns/health-checks/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteDNSHealthCheck))
}
