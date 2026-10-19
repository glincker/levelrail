package api

import "net/http"

func (rt *Router) registerPrivateNetworkRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/private-networks", rt.requireAbility(AbilityRead, rt.handleListPrivateNetworks))
	mux.HandleFunc("POST /api/v1/private-networks", rt.requireAbility(AbilityRoot, rt.handleCreatePrivateNetwork))
	mux.HandleFunc("GET /api/v1/private-networks/map", rt.requireAbility(AbilityRead, rt.handlePrivateNetworkMap))
	mux.HandleFunc("POST /api/v1/private-networks/apply", rt.requireAbility(AbilityRoot, rt.handleApplyPrivateNetworks))
	mux.HandleFunc("POST /api/v1/private-networks/rules", rt.requireAbility(AbilityRoot, rt.handleCreatePrivateNetworkRule))
	mux.HandleFunc("DELETE /api/v1/private-networks/rules/{id}", rt.requireAbility(AbilityRoot, rt.handleDeletePrivateNetworkRule))
	mux.HandleFunc("PUT /api/v1/private-networks/{id}", rt.requireAbility(AbilityRoot, rt.handleUpdatePrivateNetwork))
	mux.HandleFunc("DELETE /api/v1/private-networks/{id}", rt.requireAbility(AbilityRoot, rt.handleDeletePrivateNetwork))
}
