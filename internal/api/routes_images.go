package api

import "net/http"

// registerImageRoutes wires the image inventory, layer breakdown, retention
// GC and signature verification.
func (rt *Router) registerImageRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/images", rt.requireAbility(AbilityRead, rt.handleListImageInventory))
	mux.HandleFunc("GET /api/v1/images/layers", rt.requireAbility(AbilityRead, rt.handleImageLayerBreakdown))
	mux.HandleFunc("POST /api/v1/images/verify", rt.requireAbility(AbilityWrite, rt.handleImageVerify))
	mux.HandleFunc("POST /api/v1/images/gc", rt.requireAbility(AbilityRoot, rt.handleImageGC))
}
