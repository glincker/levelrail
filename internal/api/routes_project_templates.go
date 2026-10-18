package api

import "net/http"

func (rt *Router) registerProjectTemplateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/project-templates", rt.requireAbility(AbilityRead, rt.handleListProjectTemplates))
	// A POST that only validates parameters and renders text, so AbilityRead.
	mux.HandleFunc("POST /api/v1/project-templates/{id}/render", rt.requireAbility(AbilityRead, rt.handleRenderProjectTemplate))
}
