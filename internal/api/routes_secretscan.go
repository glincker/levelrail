package api

import "net/http"

func (rt *Router) registerSecretScanRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/secret-scan", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetSecretScan))
	mux.HandleFunc("PUT /api/v1/apps/{name}/secret-scan", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handlePutSecretScan))
	mux.HandleFunc("POST /api/v1/apps/{name}/secret-scan/run", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleRunSecretScan))
	mux.HandleFunc("POST /api/v1/apps/{name}/secret-scan/allow", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleAllowSecretScan))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/secret-scan/allow/{fingerprint}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleUnallowSecretScan))
	mux.HandleFunc("GET /api/v1/apps/{name}/deployments/{id}/secret-scan", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDeploymentSecretScan))
}
