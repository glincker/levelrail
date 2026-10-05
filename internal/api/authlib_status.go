package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/authengine"
)

type authEngineStatusResponse struct {
	LibraryVersion string `json:"library_version"`
	TOTP           bool   `json:"totp"`
	Passkeys       bool   `json:"passkeys"`
	OAuth          bool   `json:"oauth"`
}

func (rt *Router) registerAuthEngineStatusRoute(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/auth-engine/status", rt.requireAbility(AbilityRoot, rt.handleAuthEngineStatus))
}

// handleAuthEngineStatus reports the auth library version and which optional
// features this control plane has available.
func (rt *Router) handleAuthEngineStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, authEngineStatusResponse{
		LibraryVersion: authengine.LibraryVersion(),
		TOTP:           rt.mfaLib.mfa.TOTPAvailable(),
		Passkeys:       rt.mfaLib.mfa.PasskeysAvailable(),
		OAuth:          rt.authLibOAuthActive(),
	})
}
