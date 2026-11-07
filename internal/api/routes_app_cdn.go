package api

import "net/http"

// registerAppCDNRoutes wires the per-app Cloudflare proxy override. Changing
// it alters how visitors reach the site, so it needs AbilityWriteSensitive.
func (rt *Router) registerAppCDNRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/cdn", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppCDN))
	mux.HandleFunc("PUT /api/v1/apps/{name}/cdn", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handlePutAppCDN))
	mux.HandleFunc("POST /api/v1/apps/{name}/cdn/resync", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleResyncAppCDN))
	rt.registerCDNEdgeRoutes(mux)
}
