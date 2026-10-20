package api

import "net/http"

func (rt *Router) registerMobileRoutes(mux *http.ServeMux) {
	// Minting a pairing code hands out a credential, so root plus a fresh step-up.
	mux.HandleFunc("POST /api/v1/mobile/pairing-codes", rt.requireAbility(AbilityRoot, rt.handleCreateMobilePairing))
	// Unauthenticated by design: the one-time code is the credential, rate limited.
	mux.HandleFunc("POST /api/v1/mobile/pair", rt.handleMobilePair)
	mux.HandleFunc("GET /api/v1/mobile/devices", rt.requireAbility(AbilityRead, rt.handleListMobileDevices))
	// Read tier so a device owner (or the phone itself) can revoke; root or owner is checked inside.
	mux.HandleFunc("DELETE /api/v1/mobile/devices/{id}", rt.requireAbility(AbilityRead, rt.handleRevokeMobileDevice))
	mux.HandleFunc("GET /api/v1/mobile/overview", rt.requireAbility(AbilityRead, rt.handleMobileOverview))
}
