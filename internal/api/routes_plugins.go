package api

import "net/http"

func (rt *Router) registerPluginRoutes(mux *http.ServeMux) {
	// Plugins run third-party code beside the control plane, so every route is AbilityRoot.
	mux.HandleFunc("GET /api/v1/plugins", rt.requireAbility(AbilityRoot, rt.handleListPlugins))
	mux.HandleFunc("GET /api/v1/plugins/{name}", rt.requireAbility(AbilityRoot, rt.handleGetPlugin))
	mux.HandleFunc("POST /api/v1/plugins", rt.requireAbility(AbilityRoot, rt.handleInstallPlugin))
	mux.HandleFunc("POST /api/v1/plugins/{name}/approve", rt.requireAbility(AbilityRoot, rt.handleApprovePlugin))
	mux.HandleFunc("POST /api/v1/plugins/{name}/enable", rt.requireAbility(AbilityRoot, rt.handleEnablePlugin))
	mux.HandleFunc("POST /api/v1/plugins/{name}/disable", rt.requireAbility(AbilityRoot, rt.handleDisablePlugin))
	mux.HandleFunc("DELETE /api/v1/plugins/{name}", rt.requireAbility(AbilityRoot, rt.handleRemovePlugin))
	mux.HandleFunc("GET /api/v1/plugin-keys", rt.requireAbility(AbilityRoot, rt.handleListPluginKeys))
	mux.HandleFunc("POST /api/v1/plugin-keys", rt.requireAbility(AbilityRoot, rt.handleAddPluginKey))
	mux.HandleFunc("DELETE /api/v1/plugin-keys/{id}", rt.requireAbility(AbilityRoot, rt.handleRemovePluginKey))
}
