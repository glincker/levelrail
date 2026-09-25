package api

import "net/http"

// registerModelRoutes registers the AI model and GPU routes. Reads are
// AbilityRead; create, key rotation and HuggingFace token writes touch
// credentials so they are AbilityWriteSensitive; delete and restart are
// AbilityWrite.
func (rt *Router) registerModelRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/models", rt.requireAbility(AbilityRead, rt.handleListModels))
	mux.HandleFunc("POST /api/v1/models", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateModel))
	mux.HandleFunc("GET /api/v1/models/{name}", rt.requireAbilityForResource(AbilityRead, modelResourceFromPath, rt.handleGetModel))
	mux.HandleFunc("DELETE /api/v1/models/{name}", rt.requireAbilityForResource(AbilityWrite, modelResourceFromPath, rt.handleDeleteModel))
	mux.HandleFunc("POST /api/v1/models/{name}/restart", rt.requireAbilityForResource(AbilityWrite, modelResourceFromPath, rt.handleRestartModel))
	mux.HandleFunc("POST /api/v1/models/{name}/api-key", rt.requireAbilityForResource(AbilityWriteSensitive, modelResourceFromPath, rt.handleRotateModelAPIKey))
	mux.HandleFunc("PUT /api/v1/models/{name}/hf-token", rt.requireAbilityForResource(AbilityWriteSensitive, modelResourceFromPath, rt.handleSetModelHFToken))
	mux.HandleFunc("GET /api/v1/models/{name}/logs", rt.requireAbilityForResource(AbilityRead, modelResourceFromPath, rt.handleQueryModelLogs))
	mux.HandleFunc("GET /api/v1/models/{name}/logs/stream", rt.requireAbilityForResource(AbilityRead, modelResourceFromPath, rt.handleLiveModelLogStream))
	mux.HandleFunc("GET /api/v1/gpus", rt.requireAbility(AbilityRead, rt.handleListGPUNodes))
}
