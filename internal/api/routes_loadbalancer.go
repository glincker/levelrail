package api

import "net/http"

// registerLoadBalancerRoutes: per-app load balancer config, live status
// and IaC export. Reads need AbilityRead, changes AbilityWrite.
func (rt *Router) registerLoadBalancerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/loadbalancers", rt.requireAbility(AbilityRead, rt.handleListLoadBalancers))
	mux.HandleFunc("GET /api/v1/apps/{name}/loadbalancer", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetLoadBalancer))
	mux.HandleFunc("PUT /api/v1/apps/{name}/loadbalancer", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSetLoadBalancer))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/loadbalancer", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeleteLoadBalancer))
	mux.HandleFunc("POST /api/v1/apps/{name}/loadbalancer/import", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleImportLoadBalancer))
	mux.HandleFunc("GET /api/v1/apps/{name}/loadbalancer/status", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleLoadBalancerStatus))
	mux.HandleFunc("GET /api/v1/apps/{name}/loadbalancer/export", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleExportLoadBalancer))
}
