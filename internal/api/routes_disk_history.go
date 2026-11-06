package api

import "net/http"

// registerDiskHistoryRoutes registers the storage trend feed, AbilityRead like system/status.
func (rt *Router) registerDiskHistoryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/system/disk/history", rt.requireAbility(AbilityRead, rt.handleDiskHistory))
}
