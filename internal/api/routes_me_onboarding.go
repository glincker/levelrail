package api

import "net/http"

func (rt *Router) registerMeOnboardingRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/me/onboarding", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleGetMeOnboarding))
	mux.HandleFunc("PUT /api/v1/me/onboarding", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handlePutMeOnboarding))
	mux.HandleFunc("POST /api/v1/me/onboarding/reset", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleResetMeOnboarding))
}
