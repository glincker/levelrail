package api

import "net/http"

func (rt *Router) registerSetupGuideRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/setup/guide", rt.requireAbility(AbilityRead, rt.handleGetSetupGuide))
	mux.HandleFunc("PUT /api/v1/setup/guide/steps/{id}", rt.requireAbility(AbilityRoot, rt.handleSetSetupGuideStep))
	mux.HandleFunc("PUT /api/v1/setup/guide/visibility", rt.requireAbility(AbilityRoot, rt.handleSetSetupGuideVisibility))
	mux.HandleFunc("POST /api/v1/setup/guide/reset", rt.requireAbility(AbilityRoot, rt.handleResetSetupGuide))
}
