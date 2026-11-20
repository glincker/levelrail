package api

import "net/http"

func (rt *Router) registerBuildPerformanceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/builds", rt.requireAbility(AbilityRead, rt.handleListBuilds))
	mux.HandleFunc("GET /api/v1/builds/cache", rt.requireAbility(AbilityRead, rt.handleBuildCache))
	// Pruning frees disk shared by every app's builds, so write-sensitive even as a preview.
	mux.HandleFunc("POST /api/v1/builds/cache/prune", rt.requireAbility(AbilityWriteSensitive, rt.handleBuildCachePrune))
	// The report filters reclaims to the caller's apps; budgets change through runtime settings.
	mux.HandleFunc("GET /api/v1/builds/cache/quota", rt.requireAbility(AbilityRead, rt.handleBuildCacheQuota))
	mux.HandleFunc("POST /api/v1/apps/{name}/build-cache/reclaim", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleReclaimAppBuildCache))
	mux.HandleFunc("GET /api/v1/builds/runners", rt.requireAbility(AbilityRead, rt.handleListBuildRunners))
	mux.HandleFunc("GET /api/v1/apps/{name}/build-runner", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppBuildRunner))
	// Choosing where an app builds changes where its source and secrets are read, so deploy rights on the app.
	mux.HandleFunc("PUT /api/v1/apps/{name}/build-runner", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetAppBuildRunner))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/build-runner", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleClearAppBuildRunner))
	// A node's tier and an environment's allow-list decide whose builds may reach production, so sensitive writes.
	mux.HandleFunc("PUT /api/v1/nodes/{id}/build-trust", rt.requireAbility(AbilityRoot, rt.handleSetNodeBuildTrust))
	mux.HandleFunc("GET /api/v1/environments/{id}/builders", rt.requireAbility(AbilityRead, rt.requireEnvironmentGrant(AbilityRead, rt.handleGetEnvironmentBuilders)))
	mux.HandleFunc("PUT /api/v1/environments/{id}/builders", rt.requireAuth(rt.requireEnvironmentGrant(AbilityRead, rt.requireAbility(AbilityWriteSensitive, rt.handleSetEnvironmentBuilders))))
	mux.HandleFunc("GET /api/v1/builds/queue", rt.requireAbility(AbilityRead, rt.handleGetBuildQueue))
	// Holding or killing builds stops every app's deploys, so write-sensitive; kill-all also needs step-up.
	mux.HandleFunc("POST /api/v1/builds/queue/pause", rt.requireAbility(AbilityWriteSensitive, rt.handleBuildQueuePause))
	mux.HandleFunc("POST /api/v1/builds/queue/resume", rt.requireAbility(AbilityWriteSensitive, rt.handleBuildQueueResume))
	mux.HandleFunc("POST /api/v1/builds/queue/drain", rt.requireAbility(AbilityWriteSensitive, rt.handleBuildQueueDrain))
	mux.HandleFunc("POST /api/v1/builds/kill-all", rt.requireAbility(AbilityWriteSensitive, rt.handleKillAllBuilds))
	mux.HandleFunc("GET /api/v1/builds/{id}", rt.requireAbility(AbilityRead, rt.handleGetBuild))
	// The record names an app's builder and digest, so it is hidden unless the caller can see the app's environment.
	mux.HandleFunc("GET /api/v1/builds/{id}/record", rt.requireAbility(AbilityRead, rt.handleGetBuildRecord))
	mux.HandleFunc("POST /api/v1/apps/{name}/builds/{id}/cancel", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleCancelBuild))
	mux.HandleFunc("POST /api/v1/apps/{name}/builds/{id}/retry", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleRetryBuild))
	mux.HandleFunc("GET /api/v1/apps/{name}/build-stats", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleBuildStats))
	mux.HandleFunc("GET /api/v1/apps/{name}/build-speed", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleBuildSpeed))
	mux.HandleFunc("GET /api/v1/build-limits", rt.requireAbility(AbilityRead, rt.handleGetBuildLimits))
	// Limits decide when every app's builds are killed, so root only.
	mux.HandleFunc("PUT /api/v1/build-limits", rt.requireAbility(AbilityRoot, rt.handleSetBuildLimits))
	mux.HandleFunc("DELETE /api/v1/build-limits", rt.requireAbility(AbilityRoot, rt.handleResetBuildLimits))
	mux.HandleFunc("GET /api/v1/apps/{name}/build-limits", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppBuildLimits))
	// An app's CPU and memory cap decides how much of the host its builds can take, so root only.
	mux.HandleFunc("PUT /api/v1/apps/{name}/build-limits", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleSetAppBuildLimits))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/build-limits", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleResetAppBuildLimits))
}
