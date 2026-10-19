package api

import "net/http"

func (rt *Router) registerPluginRoutes(mux *http.ServeMux) {
	// Plugins run third-party code beside the control plane, so lifecycle routes are AbilityRoot.
	// Panel reads and button presses only reach a plugin through its declared, approved source.
	mux.HandleFunc("GET /api/v1/plugins", rt.requireAbility(AbilityRoot, rt.handleListPlugins))
	mux.HandleFunc("GET /api/v1/plugins/{name}", rt.requireAbility(AbilityRoot, rt.handleGetPlugin))
	mux.HandleFunc("POST /api/v1/plugins", rt.requireAbility(AbilityRoot, rt.handleInstallPlugin))
	mux.HandleFunc("POST /api/v1/plugins/inspect", rt.requireAbility(AbilityRoot, rt.handleInspectPlugin))
	mux.HandleFunc("POST /api/v1/plugins/{name}/upgrade", rt.requireAbility(AbilityRoot, rt.handleUpgradePlugin))
	mux.HandleFunc("POST /api/v1/plugins/{name}/restart", rt.requireAbility(AbilityRoot, rt.handleRestartPlugin))
	mux.HandleFunc("POST /api/v1/plugins/{name}/approve", rt.requireAbility(AbilityRoot, rt.handleApprovePlugin))
	mux.HandleFunc("POST /api/v1/plugins/{name}/enable", rt.requireAbility(AbilityRoot, rt.handleEnablePlugin))
	mux.HandleFunc("POST /api/v1/plugins/{name}/disable", rt.requireAbility(AbilityRoot, rt.handleDisablePlugin))
	mux.HandleFunc("DELETE /api/v1/plugins/{name}", rt.requireAbility(AbilityRoot, rt.handleRemovePlugin))
	mux.HandleFunc("GET /api/v1/plugin-panels", rt.requireAbility(AbilityRead, rt.handleListPluginPanels))
	mux.HandleFunc("GET /api/v1/plugins/{name}/panels/{panel}/data", rt.requireAbility(AbilityRead, rt.handlePluginPanelData))
	mux.HandleFunc("POST /api/v1/plugins/{name}/panels/{panel}/actions/{action}", rt.requireAbility(AbilityWriteSensitive, rt.handlePluginPanelAction))
	mux.HandleFunc("GET /api/v1/plugin-keys", rt.requireAbility(AbilityRoot, rt.handleListPluginKeys))
	mux.HandleFunc("POST /api/v1/plugin-keys", rt.requireAbility(AbilityRoot, rt.handleAddPluginKey))
	mux.HandleFunc("DELETE /api/v1/plugin-keys/{id}", rt.requireAbility(AbilityRoot, rt.handleRemovePluginKey))
	mux.HandleFunc("GET /api/v1/plugin-marketplace", rt.requireAbility(AbilityRoot, rt.handleMarketplaceCatalog))
	mux.HandleFunc("POST /api/v1/plugin-marketplace/refresh", rt.requireAbility(AbilityRoot, rt.handleMarketplaceRefresh))
	mux.HandleFunc("POST /api/v1/plugin-marketplace/preview", rt.requireAbility(AbilityRoot, rt.handleMarketplacePreview))
	mux.HandleFunc("POST /api/v1/plugin-marketplace/install", rt.requireAbility(AbilityRoot, rt.handleMarketplaceInstall))
	mux.HandleFunc("GET /api/v1/plugin-marketplace/sources", rt.requireAbility(AbilityRoot, rt.handleMarketplaceSources))
	mux.HandleFunc("POST /api/v1/plugin-marketplace/sources", rt.requireAbility(AbilityRoot, rt.handleMarketplaceAddSource))
	mux.HandleFunc("DELETE /api/v1/plugin-marketplace/sources/{id}", rt.requireAbility(AbilityRoot, rt.handleMarketplaceRemoveSource))
}
