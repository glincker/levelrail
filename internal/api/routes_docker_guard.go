package api

import "net/http"

func (rt *Router) registerDockerGuardRoutes(mux *http.ServeMux) {
	// Docker API guard status (Security settings page): AbilityRead like system/doctor.
	mux.HandleFunc("GET /api/v1/system/docker-guard", rt.requireAbility(AbilityRead, rt.handleGetDockerGuard))
	// Switching modes changes the host-protection boundary, so AbilityRoot.
	mux.HandleFunc("PUT /api/v1/system/docker-guard", rt.requireAbility(AbilityRoot, rt.handleUpdateDockerGuard))
	mux.HandleFunc("GET /api/v1/mesh/settings", rt.requireAbility(AbilityRead, rt.handleGetMeshSettings))
	// Turning the mesh on or off changes how every node talks to this host, so AbilityRoot.
	mux.HandleFunc("PUT /api/v1/mesh/settings", rt.requireAbility(AbilityRoot, rt.handleUpdateMeshSettings))
}
