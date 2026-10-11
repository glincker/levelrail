package api

import "net/http"

func (rt *Router) registerDatabaseUpgradeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/databases/upgrade-summary", rt.requireAbility(AbilityRead, rt.handleDatabaseUpgradeSummary))
	mux.HandleFunc("GET /api/v1/databases/{name}/upgrades", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleGetDatabaseUpgrades))
	// Policy and upgrade-now restart the database, the same tier as PUT .../version.
	mux.HandleFunc("PUT /api/v1/databases/{name}/upgrade-policy", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handlePutDatabaseUpgradePolicy))
	mux.HandleFunc("POST /api/v1/databases/{name}/upgrade-now", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleDatabaseUpgradeNow))
	// The platform default reaches every database, so writing it is root-only.
	mux.HandleFunc("GET /api/v1/settings/database-upgrades", rt.requireAbility(AbilityRead, rt.handleGetPlatformUpgradePolicy))
	mux.HandleFunc("PUT /api/v1/settings/database-upgrades", rt.requireAbility(AbilityRoot, rt.handlePutPlatformUpgradePolicy))
}
