package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
)

type rotateMasterKeyRequest struct {
	NewMasterKey string `json:"newMasterKey"`
}

type rotateMasterKeyResponse struct {
	RotatedAt time.Time `json:"rotatedAt"`
	// PersistedToFile is true once the new key was also written to disk
	// (masterKeyFilePath), so a restart of this control plane picks it
	// up automatically. False when the master key is env-sourced
	// (APP_MASTER_KEY) or the file write itself failed: Warning explains
	// which, and either way the operator has a required follow-up
	// before the next restart.
	PersistedToFile bool   `json:"persistedToFile"`
	Warning         string `json:"warning,omitempty"`
}

// handleRotateMasterKey handles POST /api/v1/system/master-key/rotate:
// re-wraps every stored DEK from the currently active master key to the
// one supplied in the request body, live, without restarting the
// control plane. AbilityRoot (routes.go): this is the single key every
// other secret in this control plane depends on, not a per-app or
// per-token-scoped credential.
//
// 501 without a MasterKeyRotator configured, the same "not configured"
// shape every other optional-dependency route in this package uses. 400
// covers both a missing/malformed request body and a rotation the
// rotator itself rejected (most likely a corrupt stored DEK, surfaced
// as the rotator's own error text) rather than 500, since neither case
// is this handler's own fault and the response needs to explain why
// clearly enough for an operator to act on.
func (rt *Router) handleRotateMasterKey(w http.ResponseWriter, r *http.Request) {
	if rt.masterKeyRotator == nil {
		writeError(w, http.StatusNotImplemented, "master key rotation is not configured on this control plane (no master key loaded)")
		return
	}

	var req rotateMasterKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	newKey := strings.TrimSpace(req.NewMasterKey)
	if newKey == "" {
		writeError(w, http.StatusBadRequest, "newMasterKey is required")
		return
	}

	rotatedAt, err := rt.masterKeyRotator.RotateMasterKey(r.Context(), newKey)
	if err != nil {
		rt.logger.Error("api: rotate master key failed", slog.String("error", err.Error()))
		writeError(w, http.StatusBadRequest, "rotate master key: "+err.Error())
		return
	}

	resp := rotateMasterKeyResponse{RotatedAt: rotatedAt}
	if rt.masterKeyFilePath == "" {
		resp.Warning = "the master key is sourced from APP_MASTER_KEY: update that environment variable to the new value before this control plane is next restarted, or it will fail to decrypt every stored secret on startup"
	} else if err := secrets.PersistMasterKeyFile(rt.masterKeyFilePath, newKey); err != nil {
		rt.logger.Error("api: persist rotated master key file failed", slog.String("error", err.Error()), slog.String("path", rt.masterKeyFilePath))
		resp.Warning = "the master key was rotated successfully but could not be written to " + rt.masterKeyFilePath + ": update that file by hand with the new key before this control plane is next restarted, or it will fail to decrypt every stored secret on startup"
	} else {
		resp.PersistedToFile = true
	}

	writeJSON(w, http.StatusOK, resp)
}
