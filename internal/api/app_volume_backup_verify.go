package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// handleVerifyVolumeBackup handles
// POST /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/verify,
// the volume counterpart of handleVerifyBackup (backup_verify.go):
// reuses the exact same rt.backupVerifier (internal/backup.VerifyRunner
// is already resource-agnostic), passing engine "" since a volume has no
// database engine, the same "no engine-specific magic-byte check
// available, checksum/size checks still run" reasoning
// Scheduler.runScheduledVolume's own doc comment gives for the identical
// choice on the scheduled path.
func (rt *Router) handleVerifyVolumeBackup(w http.ResponseWriter, r *http.Request) {
	if rt.backupVerifier == nil {
		writeError(w, http.StatusNotImplemented, "backup verification is not configured on this control plane (no master key set)")
		return
	}

	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")
	historyID := r.PathValue("historyId")

	if _, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: verify volume backup: load service failed"); !ok {
		return
	}

	h, ok := rt.loadVolumeBackupHistory(w, r, serviceName, volumeName, historyID, "api: verify volume backup: load backup history failed")
	if !ok {
		return
	}
	if h.Status != store.BackupStatusSucceeded {
		writeError(w, http.StatusConflict, fmt.Sprintf("backup %q has status %q, only a succeeded backup can be verified", historyID, h.Status))
		return
	}

	verificationID, err := randomBackupVerificationID()
	if err != nil {
		rt.logger.Error("api: verify volume backup: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	checkedBy := rt.checkedByFromRequest(r)

	go func() { //nolint:gosec // deliberately not r.Context(): it is cancelled the moment this handler returns, the same reasoning handleVerifyBackup's own goroutine gives
		if err := rt.backupVerifier.VerifyBackup(context.Background(), verificationID, historyID, "", checkedBy); err != nil {
			rt.logger.Error("api: volume backup verification failed", slog.String("error", err.Error()), slog.String("id", verificationID), slog.String("backup_id", historyID))
		}
	}()

	writeJSON(w, http.StatusAccepted, backupVerificationResource{
		ID:              verificationID,
		BackupHistoryID: historyID,
		Status:          store.BackupVerificationStatusRunning,
		CheckedBy:       checkedBy,
	})
}

// handleListVolumeBackupVerifications handles
// GET /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/verifications,
// reusing rt.backupVerifications as-is (ListBackupVerifications is
// already keyed by backupHistoryID alone, no resource-kind distinction
// needed): the only new work here is validating the named backup
// actually belongs to this service/volume before listing.
func (rt *Router) handleListVolumeBackupVerifications(w http.ResponseWriter, r *http.Request) {
	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")
	historyID := r.PathValue("historyId")

	if _, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: list volume backup verifications: load service failed"); !ok {
		return
	}

	if _, ok := rt.loadVolumeBackupHistory(w, r, serviceName, volumeName, historyID, "api: list volume backup verifications: load backup history failed"); !ok {
		return
	}

	rt.writeBackupVerificationsList(w, r, historyID, "api: list volume backup verifications failed")
}
