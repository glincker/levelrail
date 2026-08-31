package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// BackupVerificationStore is the store surface the backup verification
// handlers need: listing past verification attempts for one backup, the
// same "core Store interface, no runner configuration needed" shape
// BackupHistoryStore's own doc comment establishes for backup history
// itself.
type BackupVerificationStore interface {
	ListBackupVerifications(ctx context.Context, backupHistoryID string, limit int) ([]store.BackupVerification, error)
}

// BackupVerifier is the surface the verify-trigger handler needs from
// internal/backup.VerifyRunner: run one verification attempt end to end,
// from an already-minted verification ID through to a finished
// store.BackupVerification row. *backup.VerifyRunner satisfies this
// structurally; internal/api never imports internal/backup directly, the
// same boundary BackupRunner's own doc comment describes.
type BackupVerifier interface {
	VerifyBackup(ctx context.Context, verificationID, backupHistoryID, engine, checkedBy string) error
}

// backupVerificationResource is the wire shape for one verification
// attempt.
type backupVerificationResource struct {
	ID              string `json:"id"`
	BackupHistoryID string `json:"backup_history_id"`
	Status          string `json:"status"`
	ChecksumMatch   bool   `json:"checksum_match"`
	SizeMatch       bool   `json:"size_match"`
	FormatValid     bool   `json:"format_valid"`
	DownloadedBytes int64  `json:"downloaded_bytes"`
	Error           string `json:"error,omitempty"`
	CheckedBy       string `json:"checked_by,omitempty"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at,omitempty"`
}

func toBackupVerificationResource(v store.BackupVerification) backupVerificationResource {
	return backupVerificationResource{
		ID:              v.ID,
		BackupHistoryID: v.BackupHistoryID,
		Status:          v.Status,
		ChecksumMatch:   v.ChecksumMatch,
		SizeMatch:       v.SizeMatch,
		FormatValid:     v.FormatValid,
		DownloadedBytes: v.DownloadedBytes,
		Error:           v.Error,
		CheckedBy:       v.CheckedBy,
		StartedAt:       v.StartedAt,
		FinishedAt:      v.FinishedAt,
	}
}

// handleVerifyBackup handles
// POST /api/v1/databases/{name}/backups/{historyId}/verify: re-downloads a
// previously succeeded backup's stored object and checks it for
// corruption (checksum, size, and a lightweight structural check, see
// internal/backup.VerifyRunner's own doc comment), returning as soon as
// the attempt is recorded and under way (StatusAccepted, the same
// "returns once desired state is saved, not once it has actually
// converged" shape handleTriggerBackup already establishes). GET
// .../verifications (handleListBackupVerifications below) is how a
// caller finds out whether it actually passed.
//
// Deliberately never attempts a live restore against a running database:
// that is real risk this codebase's own scope for this feature explicitly
// rules out for an automated check (see VerifyRunner's own doc comment).
//
// AbilityWriteSensitive, matching handleTriggerBackup exactly: this
// starts real work (a real download from a real bucket using a
// previously-stored credential) and persists a new store.BackupVerification
// row, the same sensitivity class triggering an ordinary backup already
// carries, even though unlike a download this never returns the backup's
// own bytes to the caller.
func (rt *Router) handleVerifyBackup(w http.ResponseWriter, r *http.Request) {
	if rt.backupVerifier == nil {
		writeError(w, http.StatusNotImplemented, "backup verification is not configured on this control plane (no master key set)")
		return
	}

	name := r.PathValue("name")
	historyID := r.PathValue("historyId")

	db, ok := rt.loadDatabaseForRunner(w, r, name, "api: verify backup: load database failed")
	if !ok {
		return
	}

	h, err := rt.backupHistory.GetBackupHistory(r.Context(), historyID)
	if errors.Is(err, store.ErrBackupHistoryNotFound) {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: verify backup: load backup history failed", slog.String("error", err.Error()), slog.String("backup_id", historyID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if h.DatabaseName != name {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("backup %q was taken from database %q, not %q", historyID, h.DatabaseName, name))
		return
	}
	if h.Status != store.BackupStatusSucceeded {
		writeError(w, http.StatusConflict, fmt.Sprintf("backup %q has status %q, only a succeeded backup can be verified", historyID, h.Status))
		return
	}

	verificationID, err := randomBackupVerificationID()
	if err != nil {
		rt.logger.Error("api: verify backup: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	checkedBy := rt.checkedByFromRequest(r)

	go func() { //nolint:gosec // deliberately not r.Context(): it is cancelled the moment this handler returns, which would abort the verification within microseconds of starting it, the same reasoning handleTriggerBackup's own goroutine gives
		if err := rt.backupVerifier.VerifyBackup(context.Background(), verificationID, historyID, db.Engine, checkedBy); err != nil {
			rt.logger.Error("api: backup verification failed", slog.String("error", err.Error()), slog.String("id", verificationID), slog.String("backup_id", historyID))
		}
	}()

	writeJSON(w, http.StatusAccepted, backupVerificationResource{
		ID:              verificationID,
		BackupHistoryID: historyID,
		Status:          store.BackupVerificationStatusRunning,
		CheckedBy:       checkedBy,
	})
}

// handleListBackupVerifications handles
// GET /api/v1/databases/{name}/backups/{historyId}/verifications, newest
// first. AbilityRead, matching handleListBackupHistory: passive
// visibility into attempts already made, never touches the bucket.
func (rt *Router) handleListBackupVerifications(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	historyID := r.PathValue("historyId")

	h, err := rt.backupHistory.GetBackupHistory(r.Context(), historyID)
	if errors.Is(err, store.ErrBackupHistoryNotFound) {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: list backup verifications: load backup history failed", slog.String("error", err.Error()), slog.String("backup_id", historyID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if h.DatabaseName != name {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("backup %q was taken from database %q, not %q", historyID, h.DatabaseName, name))
		return
	}

	verifications, err := rt.backupVerifications.ListBackupVerifications(r.Context(), historyID, defaultBackupHistoryLimit)
	if err != nil {
		rt.logger.Error("api: list backup verifications failed", slog.String("error", err.Error()), slog.String("backup_id", historyID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]backupVerificationResource, 0, len(verifications))
	for _, v := range verifications {
		out = append(out, toBackupVerificationResource(v))
	}
	writeJSON(w, http.StatusOK, out)
}

// checkedByFromRequest resolves a human-readable label for whoever is
// calling handleVerifyBackup, stored directly on the resulting
// store.BackupVerification row so "who last checked this backup" is
// visible without cross-referencing the audit log (requireAbility's own
// audit hook already records the identical actor there for every
// AbilityWriteSensitive call, this just makes it visible on the
// verification record itself too). Best-effort: an unresolvable actor
// degrades to "unknown" rather than failing the request, the same
// contract auditActorName's own doc comment establishes for the audit
// log's equivalent lookup.
func (rt *Router) checkedByFromRequest(r *http.Request) string {
	if userID, ok := rt.currentSessionUserID(r); ok {
		user, err := rt.auth.GetUserByID(r.Context(), userID)
		if err != nil {
			return userID
		}
		return user.DisplayName
	}
	if token, ok := bearerToken(r); ok {
		rec, err := rt.tokens.GetAPITokenByHash(r.Context(), hashToken(token))
		if err == nil {
			return rec.Name
		}
	}
	return "unknown"
}

// randomBackupVerificationID mirrors randomBackupHistoryID's exact shape,
// the same "different resource, different ID space" reasoning that
// function's own doc comment gives, with its own "bkv_" prefix so a
// verification ID and the backup history ID it names are never visually
// confusable.
func randomBackupVerificationID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate backup verification id: %w", err)
	}
	return "bkv_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
