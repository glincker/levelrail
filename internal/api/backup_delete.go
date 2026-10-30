package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// BackupDeleter is the delete surface internal/api needs from internal/backup, so internal/api never imports that package.
type BackupDeleter interface {
	DeleteBackup(ctx context.Context, historyID string) error
}

// refuseDeletingRunningBackup refuses to delete a running backup, whose runner still writes to the row.
// Succeeded or failed attempts can always be deleted.
func refuseDeletingRunningBackup(w http.ResponseWriter, historyID string, status string) bool {
	if status != store.BackupStatusRunning {
		return false
	}
	writeError(w, http.StatusConflict, fmt.Sprintf("backup %q has status %q, cannot delete a backup that is still running", historyID, status))
	return true
}

// handleDeleteBackup deletes one archived backup on demand. The stored object is removed best-effort first, then the history row.
// Gated AbilityWriteSensitive, not AbilityRoot, since no live database data is touched.
func (rt *Router) handleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	if rt.backupDeleter == nil {
		writeError(w, http.StatusNotImplemented, "backup deletion is not configured on this control plane (no master key set)")
		return
	}

	name := r.PathValue("name")
	historyID := r.PathValue("historyId")

	h, ok := rt.loadDatabaseBackupHistory(w, r, name, historyID, "api: delete backup: load backup history failed")
	if !ok {
		return
	}
	if refuseDeletingRunningBackup(w, historyID, h.Status) {
		return
	}

	if err := rt.backupDeleter.DeleteBackup(r.Context(), historyID); err != nil {
		if errors.Is(err, store.ErrBackupInUse) {
			writeError(w, http.StatusConflict, fmt.Sprintf("backup %q cannot be deleted: it was used by a restore or clone (%s); the restore history is kept", historyID, err.Error()))
			return
		}
		rt.internalError(w, "api: delete backup failed", err, slog.String("backup_id", historyID), slog.String("database", name))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteVolumeBackup is the app-volume counterpart of handleDeleteBackup, with the same behavior and ability gate.
func (rt *Router) handleDeleteVolumeBackup(w http.ResponseWriter, r *http.Request) {
	if rt.backupDeleter == nil {
		writeError(w, http.StatusNotImplemented, "backup deletion is not configured on this control plane (no master key set)")
		return
	}

	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")
	historyID := r.PathValue("historyId")

	if _, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: delete volume backup: load service failed"); !ok {
		return
	}

	h, ok := rt.loadVolumeBackupHistory(w, r, serviceName, volumeName, historyID, "api: delete volume backup: load backup history failed")
	if !ok {
		return
	}
	if refuseDeletingRunningBackup(w, historyID, h.Status) {
		return
	}

	if err := rt.backupDeleter.DeleteBackup(r.Context(), historyID); err != nil {
		if errors.Is(err, store.ErrBackupInUse) {
			writeError(w, http.StatusConflict, fmt.Sprintf("backup %q cannot be deleted: it was used by a restore or clone (%s); the restore history is kept", historyID, err.Error()))
			return
		}
		rt.internalError(w, "api: delete volume backup failed", err, slog.String("backup_id", historyID), slog.String("service", serviceName), slog.String("volume", volumeName))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
