package api

import "net/http"

func (rt *Router) registerDockerGuardRoutes(mux *http.ServeMux) {
	// Docker API guard status (Security settings page): AbilityRead like system/doctor.
	mux.HandleFunc("GET /api/v1/system/docker-guard", rt.requireAbility(AbilityRead, rt.handleGetDockerGuard))
	// Switching modes changes the host-protection boundary, so AbilityRoot.
	mux.HandleFunc("PUT /api/v1/system/docker-guard", rt.requireAbility(AbilityRoot, rt.handleUpdateDockerGuard))
}
