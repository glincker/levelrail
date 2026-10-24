package api

import "net/http"

func (rt *Router) registerRuntimeSettingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/settings/runtime", rt.requireAbility(AbilityRead, rt.handleListRuntimeSettings))
	mux.HandleFunc("GET /api/v1/settings/runtime/onboarding", rt.requireAbility(AbilityRead, rt.handleGetRuntimeOnboarding))
	mux.HandleFunc("GET /api/v1/settings/runtime/{key}", rt.requireAbility(AbilityRead, rt.handleGetRuntimeSetting))
	// Settings gate security protections and can restart the process, so writes are AbilityRoot.
	mux.HandleFunc("PUT /api/v1/settings/runtime/{key}", rt.requireAbility(AbilityRoot, rt.handleSetRuntimeSetting))
	mux.HandleFunc("DELETE /api/v1/settings/runtime/{key}", rt.requireAbility(AbilityRoot, rt.handleResetRuntimeSetting))
	mux.HandleFunc("POST /api/v1/settings/runtime/apply", rt.requireAbility(AbilityRoot, rt.handleApplyRuntimeSettings))
	mux.HandleFunc("POST /api/v1/settings/runtime/onboarding/ack", rt.requireAbility(AbilityRoot, rt.handleAckRuntimeOnboarding))
}
