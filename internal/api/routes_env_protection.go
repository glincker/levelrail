package api

import "net/http"

func (rt *Router) registerEnvProtectionRoutes(mux *http.ServeMux) {
	// Step-up is session only (handlers refuse tokens and session links);
	// AbilityRead just requires a signed-in caller.
	mux.HandleFunc("GET /api/v1/auth/step-up", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleStepUpOptions))
	mux.HandleFunc("POST /api/v1/auth/step-up", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleStepUp))
	mux.HandleFunc("POST /api/v1/auth/step-up/passkey/begin", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleStepUpPasskeyBegin))
	mux.HandleFunc("POST /api/v1/auth/step-up/passkey/finish", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleStepUpPasskeyFinish))

	// Reveal and full export need read:sensitive plus a step-up (or the
	// allow policy); the handlers enforce the rest and audit every value read.
	mux.HandleFunc("POST /api/v1/apps/{name}/env/reveal", rt.requireAbilityForResource(AbilityReadSensitive, appResourceFromPath, rt.handleRevealEnv))
	mux.HandleFunc("POST /api/v1/apps/{name}/secrets/{key}/reveal", rt.requireAbilityForResource(AbilityReadSensitive, appResourceFromPath, rt.handleRevealSecret))
	mux.HandleFunc("POST /api/v1/apps/{name}/env/export", rt.requireAbilityForResource(AbilityReadSensitive, appResourceFromPath, rt.handleExportEnvRevealed))
	mux.HandleFunc("GET /api/v1/apps/{name}/env/export", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleExportEnvRedacted))

	mux.HandleFunc("GET /api/v1/apps/{name}/env/settings", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetEnvSettings))
	mux.HandleFunc("PUT /api/v1/apps/{name}/env/settings", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handlePutEnvSettings))
	mux.HandleFunc("POST /api/v1/apps/{name}/env/{key}/convert-to-secret", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleConvertEnvToSecret))
}
