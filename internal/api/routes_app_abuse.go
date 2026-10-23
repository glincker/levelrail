package api

import "net/http"

// registerAppAbuseRoutes wires the per-app ingress abuse defense. Reading the
// policy needs AbilityRead; changing it can turn real visitors away, so it
// needs AbilityWriteSensitive (root, or a policy that grants it on the app).
// The dry run replays recent client addresses, so it needs AbilityReadSensitive.
func (rt *Router) registerAppAbuseRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/abuse", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppAbuse))
	mux.HandleFunc("PUT /api/v1/apps/{name}/abuse", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handlePutAppAbuse))
	// Replays the recent access log against a draft policy and counts what it would have blocked.
	mux.HandleFunc("POST /api/v1/apps/{name}/abuse/dry-run", rt.requireAbilityForResource(AbilityReadSensitive, appResourceFromPath, rt.handleAbuseDryRun))
}
