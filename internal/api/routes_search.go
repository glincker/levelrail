package api

import "net/http"

// registerSearchRoutes mounts unified search. Every hit is filtered through
// the caller's own visibility before it leaves the service (search_authz.go).
// Saved searches and usage are the caller's own and need AbilityWrite, so a
// read-only token searches but stores nothing.
func (rt *Router) registerSearchRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/search", rt.requireAbility(AbilityRead, rt.handleSearch))
	mux.HandleFunc("GET /api/v1/search/suggest", rt.requireAbility(AbilityRead, rt.handleSearchSuggest))
	mux.HandleFunc("GET /api/v1/search/saved", rt.requireAbility(AbilityRead, rt.handleListSavedSearches))
	mux.HandleFunc("POST /api/v1/search/saved", rt.requireAbility(AbilityWrite, rt.handleSaveSearch))
	mux.HandleFunc("DELETE /api/v1/search/saved/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteSavedSearch))
	mux.HandleFunc("POST /api/v1/search/usage", rt.requireAbility(AbilityWrite, rt.handleSearchUsage))
}
