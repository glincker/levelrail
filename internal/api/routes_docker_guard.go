package api

import "net/http"

func (rt *Router) registerDockerGuardRoutes(mux *http.ServeMux) {
	// Docker API guard status (Security settings page): AbilityRead like system/doctor.
	mux.HandleFunc("GET /api/v1/system/docker-guard", rt.requireAbility(AbilityRead, rt.handleGetDockerGuard))
	// Switching modes changes the host-protection boundary, so AbilityRoot.
	mux.HandleFunc("PUT /api/v1/system/docker-guard", rt.requireAbility(AbilityRoot, rt.handleUpdateDockerGuard))
	mux.HandleFunc("GET /api/v1/system/janitor", rt.requireAbility(AbilityRead, rt.handleGetJanitor))
	mux.HandleFunc("GET /api/v1/system/janitor/runs", rt.requireAbility(AbilityRead, rt.handleListJanitorRuns))
	// The policy and a run decide what gets deleted from the host, so AbilityRoot.
	mux.HandleFunc("PUT /api/v1/system/janitor/policy", rt.requireAbility(AbilityRoot, rt.handleUpdateJanitorPolicy))
	mux.HandleFunc("POST /api/v1/system/janitor/run", rt.requireAbility(AbilityRoot, rt.handleRunJanitor))
	mux.HandleFunc("GET /api/v1/system/encryption-keys", rt.requireAbility(AbilityRead, rt.handleGetEncryptionKeys))
	// Rotation rewrites every secret of an app, and the policy drives alerts: both AbilityRoot, rotation also needs step-up.
	mux.HandleFunc("PUT /api/v1/system/encryption-keys/policy", rt.requireAbility(AbilityRoot, rt.handleUpdateKeyPolicy))
	mux.HandleFunc("POST /api/v1/system/encryption-keys/rotate", rt.requireAbility(AbilityRoot, rt.handleRotateDEK))
	mux.HandleFunc("GET /api/v1/system/restart", rt.requireAbility(AbilityRead, rt.handleGetSystemRestart))
	// Exiting the process drops every in-flight request and node session, so AbilityRoot.
	mux.HandleFunc("POST /api/v1/system/restart", rt.requireAbility(AbilityRoot, rt.handleSystemRestart))
	mux.HandleFunc("GET /api/v1/local-mode", rt.requireAbility(AbilityRead, rt.handleGetLocalMode))
	// A reset deletes every app, secret and database record of the instance, so AbilityRoot.
	mux.HandleFunc("POST /api/v1/local-mode/reset", rt.requireAbility(AbilityRoot, rt.handleLocalModeReset))
	// The CLI console acts with the caller's full token, so root like the app terminal; step-up is checked inside.
	mux.HandleFunc("GET /api/v1/system/cli-console/commands", rt.requireAbility(AbilityRoot, rt.handleCLIConsoleCommands))
	mux.HandleFunc("GET /api/v1/system/cli-console/session", rt.requireAbility(AbilityRoot, rt.handleCLIConsoleSession))
	mux.HandleFunc("GET /api/v1/system/control-plane-health", rt.requireAbility(AbilityRead, rt.handleGetCPHealth))
	mux.HandleFunc("GET /api/v1/system/footprint", rt.requireAbility(AbilityRead, rt.handleGetIdleFootprint))
	// Checkpointing and ANALYZE briefly hold the single database connection, so AbilityRoot.
	mux.HandleFunc("POST /api/v1/system/control-plane-health/fix", rt.requireAbility(AbilityRoot, rt.handleCPHealthFix))
	mux.HandleFunc("GET /api/v1/system/integrity", rt.requireAbility(AbilityRead, rt.handleGetIntegrity))
	mux.HandleFunc("POST /api/v1/system/integrity/check", rt.requireAbility(AbilityRoot, rt.handleIntegrityCheck))
	// Stages a replacement of the whole control plane database for the next boot, so AbilityRoot.
	mux.HandleFunc("POST /api/v1/system/integrity/restore", rt.requireAbility(AbilityRoot, rt.handleIntegrityRestore))
	mux.HandleFunc("DELETE /api/v1/system/integrity/restore", rt.requireAbility(AbilityRoot, rt.handleIntegrityCancelRestore))
	mux.HandleFunc("GET /api/v1/system/networks", rt.requireAbility(AbilityRead, rt.handleGetSystemNetworks))
	// Removes Docker networks, so AbilityRoot.
	mux.HandleFunc("POST /api/v1/system/networks/prune", rt.requireAbility(AbilityRoot, rt.handlePruneSystemNetworks))
	mux.HandleFunc("GET /api/v1/mesh/settings", rt.requireAbility(AbilityRead, rt.handleGetMeshSettings))
	// Turning the mesh on or off changes how every node talks to this host, so AbilityRoot.
	mux.HandleFunc("PUT /api/v1/mesh/settings", rt.requireAbility(AbilityRoot, rt.handleUpdateMeshSettings))
}
