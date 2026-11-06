package api

import "net/http"

func (rt *Router) registerStorageInventoryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/system/storage/inventory", rt.requireAbility(AbilityRead, rt.handleStorageInventory))
}
