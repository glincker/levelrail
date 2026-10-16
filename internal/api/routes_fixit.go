package api

import "net/http"

func (rt *Router) registerFixitRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/fixes", rt.requireAbility(AbilityRead, rt.handleListFixes))
	mux.HandleFunc("POST /api/v1/fixes/{id}/preview", rt.requireAbility(AbilityRead, rt.handlePreviewFix))
	// Fixes prune Docker data, write settings and re-verify host state, so AbilityRoot.
	mux.HandleFunc("POST /api/v1/fixes/{id}/apply", rt.requireAbility(AbilityRoot, rt.handleApplyFix))
}
