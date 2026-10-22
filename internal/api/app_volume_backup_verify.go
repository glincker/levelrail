package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// handleVerifyVolumeBackup is the volume counterpart of handleVerifyBackup. Engine is empty since a volume has no database engine.
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
		rt.internalError(w, "api: verify volume backup: generate id failed", err)
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

// handleListVolumeBackupVerifications lists verifications for a volume backup, after checking it belongs to this volume.
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
