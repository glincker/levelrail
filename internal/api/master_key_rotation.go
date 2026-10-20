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
	// Rebind reports the legacy-value binding pass run after the rotation.
	Rebind *secretRebindResponse `json:"rebind,omitempty"`
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

	toPersist := newKey
	var wrapErr error
	if rt.masterKeyWrapper != nil {
		toPersist, wrapErr = secrets.WrapMasterKey(r.Context(), newKey, rt.masterKeyWrapper)
	}

	// Stage the new key file before the DB commit so a crash in between leaves a recoverable path.new.
	staged := false
	if rt.masterKeyFilePath != "" {
		if wrapErr != nil {
			rt.logger.Error("api: wrap rotated master key failed", slog.String("error", wrapErr.Error()))
			writeError(w, http.StatusBadGateway, "could not prepare the new master key for storage; nothing was changed")
			return
		}
		if err := secrets.StagePendingMasterKeyFile(rt.masterKeyFilePath, toPersist); err != nil {
			rt.logger.Error("api: stage rotated master key file failed", slog.String("error", err.Error()), slog.String("path", rt.masterKeyFilePath))
			writeError(w, http.StatusInternalServerError, "could not stage the new master key file; nothing was changed")
			return
		}
		staged = true
	}

	rotatedAt, err := rt.masterKeyRotator.RotateMasterKey(r.Context(), newKey)
	if err != nil {
		if staged {
			secrets.DiscardPendingMasterKeyFile(rt.masterKeyFilePath)
		}
		rt.logger.Error("api: rotate master key failed", slog.String("error", err.Error()))
		writeError(w, http.StatusBadRequest, "master key rotation failed: the new key was rejected or a stored key could not be re-wrapped; nothing was changed")
		return
	}

	resp := rotateMasterKeyResponse{RotatedAt: rotatedAt}
	switch {
	case staged:
		if err := secrets.CommitPendingMasterKeyFile(rt.masterKeyFilePath); err != nil {
			rt.logger.Error("api: commit rotated master key file failed", slog.String("error", err.Error()), slog.String("path", rt.masterKeyFilePath))
			resp.Warning = "the master key was rotated successfully but the key file swap did not finish: " + rt.masterKeyFilePath + ".new holds the new key and is applied automatically on the next start, do not delete it"
		} else {
			resp.PersistedToFile = true
		}
	case wrapErr != nil:
		rt.logger.Error("api: wrap rotated master key failed", slog.String("error", wrapErr.Error()))
		resp.Warning = "the master key was rotated successfully but could not be wrapped by the external key service, so it was not written: wrap and store the new key by hand before this control plane is next restarted, or it will fail to decrypt every stored secret on startup"
	default:
		resp.Warning = "the master key is sourced from APP_MASTER_KEY: update that environment variable to the new value before this control plane is next restarted, or it will fail to decrypt every stored secret on startup"
	}

	var rebindWarning string
	resp.Rebind, rebindWarning = rt.rebindAfterRotation(r.Context())
	if rebindWarning != "" {
		resp.Warning = strings.TrimSpace(resp.Warning + " " + rebindWarning)
	}

	writeJSON(w, http.StatusOK, resp)
}
