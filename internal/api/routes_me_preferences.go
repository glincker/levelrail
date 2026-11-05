package api

import "net/http"

func (rt *Router) registerMePreferencesRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/me/preferences/appearance", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleGetMePreferenceAppearance))
	mux.HandleFunc("PUT /api/v1/me/preferences/appearance", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handlePutMePreferenceAppearance))
	mux.HandleFunc("DELETE /api/v1/me/preferences/appearance", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleDeleteMePreferenceAppearance))
}
