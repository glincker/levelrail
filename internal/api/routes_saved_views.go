package api

import "net/http"

// Saved view handlers filter by owner and team themselves, so tenants may call them.
func init() {
	for _, k := range []string{
		"GET /api/v1/saved-views",
		"POST /api/v1/saved-views",
		"DELETE /api/v1/saved-views/{id}",
	} {
		tenantRoutes[k] = true
	}
}

// registerSavedViewsRoutes keeps patterns literal so the authz matrix and API reference can read them.
func (rt *Router) registerSavedViewsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/saved-views", rt.requireAbility(AbilityRead, rt.handleListSavedViews))
	mux.HandleFunc("POST /api/v1/saved-views", rt.requireAbility(AbilityWrite, rt.handleSaveView))
	mux.HandleFunc("DELETE /api/v1/saved-views/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteSavedView))
}
