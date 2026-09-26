package api

import "net/http"

func (rt *Router) registerSupplyChainRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/supply-chain", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetSupplyChain))
	mux.HandleFunc("PUT /api/v1/apps/{name}/supply-chain", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handlePutSupplyChain))
	mux.HandleFunc("POST /api/v1/apps/{name}/supply-chain/override", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSupplyChainOverride))
	mux.HandleFunc("GET /api/v1/apps/{name}/deployments/{id}/sbom", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetSBOM))
	mux.HandleFunc("GET /api/v1/apps/{name}/deployments/{id}/vulnerabilities", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetVulnerabilities))
	mux.HandleFunc("POST /api/v1/apps/{name}/deployments/{id}/scan", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleScanDeployment))
}
