package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

type backupTargetEncryptionStore interface {
	SetBackupTargetEncryptDumps(ctx context.Context, id string, enabled bool) error
}

type backupTargetEncryptionResource struct {
	TargetID string `json:"target_id"`
	// Enabled is whether new database dumps to this target are encrypted.
	Enabled bool `json:"enabled"`
	// KeyAvailable is whether this control plane holds a backup key to encrypt with.
	KeyAvailable bool `json:"key_available"`
	// Recipient is the public key dumps are sealed to; never the private key.
	Recipient string `json:"recipient,omitempty"`
	// KeyStoredOffBox is the operator's acknowledgement that the private key has a copy elsewhere.
	KeyStoredOffBox bool `json:"key_stored_off_box"`
}

type setBackupTargetEncryptionRequest struct {
	Enabled bool `json:"enabled"`
}

const errNoBackupKey = "this control plane has no backup encryption key (APP_BACKUP_ENCRYPTION is off), so dumps cannot be encrypted"

func (rt *Router) backupKeyAvailable() bool {
	return rt.backupProtection != nil && rt.backupProtection.Encryption().Enabled
}

// resolveCreateEncryption picks the new target's setting: on by default when a key exists, an explicit "on" without a key is refused.
func (rt *Router) resolveCreateEncryption(w http.ResponseWriter, requested *bool) (enabled, ok bool) {
	if requested == nil {
		return rt.backupKeyAvailable(), true
	}
	if *requested && !rt.backupKeyAvailable() {
		writeError(w, http.StatusConflict, errNoBackupKey)
		return false, false
	}
	return *requested, true
}

func (rt *Router) targetEncryptionResource(r *http.Request, t store.BackupTarget) backupTargetEncryptionResource {
	res := backupTargetEncryptionResource{TargetID: t.ID, Enabled: t.EncryptDumps, KeyAvailable: rt.backupKeyAvailable()}
	if res.KeyAvailable {
		enc := rt.encryptionResource(r.Context())
		res.Recipient, res.KeyStoredOffBox = enc.Recipient, enc.KeyStoredOffBox
	}
	return res
}

// handleGetBackupTargetEncryption handles GET /api/v1/backup-targets/{id}/encryption.
func (rt *Router) handleGetBackupTargetEncryption(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := rt.backupTargets.GetBackupTarget(r.Context(), id)
	if errors.Is(err, store.ErrBackupTargetNotFound) {
		writeError(w, http.StatusNotFound, "backup target not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get backup target encryption failed", err, slog.String("id", id))
		return
	}
	writeJSON(w, http.StatusOK, rt.targetEncryptionResource(r, t))
}

// handleSetBackupTargetEncryption handles PUT /api/v1/backup-targets/{id}/encryption. Root only.
// It affects new dumps only, existing objects are never rewritten.
func (rt *Router) handleSetBackupTargetEncryption(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	setter, ok := rt.backupTargets.(backupTargetEncryptionStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "backup dump encryption is not available on this control plane")
		return
	}
	var req setBackupTargetEncryptionRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if req.Enabled && !rt.backupKeyAvailable() {
		writeError(w, http.StatusConflict, errNoBackupKey)
		return
	}
	if err := setter.SetBackupTargetEncryptDumps(r.Context(), id, req.Enabled); err != nil {
		if errors.Is(err, store.ErrBackupTargetNotFound) {
			writeError(w, http.StatusNotFound, "backup target not found")
			return
		}
		rt.internalError(w, "api: set backup target encryption failed", err, slog.String("id", id))
		return
	}
	t, err := rt.backupTargets.GetBackupTarget(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: reload backup target after encryption change failed", err, slog.String("id", id))
		return
	}
	writeJSON(w, http.StatusOK, rt.targetEncryptionResource(r, t))
}
