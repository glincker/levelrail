package api

import "net/http"

// Every handler filters per resource through the caller's own scope, so tenants may call them.
func init() {
	for _, k := range []string{
		"GET /api/v1/resources",
		"POST /api/v1/resources/tags/bulk",
		"GET /api/v1/resources/views",
		"POST /api/v1/resources/views",
		"DELETE /api/v1/resources/views/{id}",
	} {
		tenantRoutes[k] = true
	}
}

func (rt *Router) registerResourceConsoleRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/resources", rt.requireAbility(AbilityRead, rt.handleListResources))
	mux.HandleFunc("POST /api/v1/resources/tags/bulk", rt.requireAbility(AbilityWrite, rt.handleBulkResourceTags))
	mux.HandleFunc("GET /api/v1/resources/views", rt.requireAbility(AbilityWrite, rt.handleListResourceViews))
	mux.HandleFunc("POST /api/v1/resources/views", rt.requireAbility(AbilityWrite, rt.handleSaveResourceView))
	mux.HandleFunc("DELETE /api/v1/resources/views/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteResourceView))
}
