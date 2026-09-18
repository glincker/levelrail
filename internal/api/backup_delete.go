package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// BackupDeleter is the surface the backup delete handlers need from
// internal/backup.DeleteRunner: delete one already-recorded backup
// attempt, both its stored object (best-effort) and its history row.
// *backup.DeleteRunner satisfies this structurally; internal/api never
// imports internal/backup directly, the same boundary BackupRunner's own
// doc comment describes.
type BackupDeleter interface {
	DeleteBackup(ctx context.Context, historyID string) error
}

// refuseDeletingRunningBackup writes a 409 for a backup that is still
// running, shared by handleDeleteBackup and handleDeleteVolumeBackup: a
// backup attempt in progress still has RunBackup/RunVolumeBackup writing
// to its row (FinishBackupHistory, runner.go), so deleting it out from
// under that write is refused outright rather than racing it. A succeeded
// or failed attempt has no such writer left and can always be deleted.
func refuseDeletingRunningBackup(w http.ResponseWriter, historyID string, status string) bool {
	if status != store.BackupStatusRunning {
		return false
	}
	writeError(w, http.StatusConflict, fmt.Sprintf("backup %q has status %q, cannot delete a backup that is still running", historyID, status))
	return true
}

// handleDeleteBackup handles
// DELETE /api/v1/databases/{name}/backups/{historyId}: permanently
// deletes one specific archived backup attempt on demand, rather than
// waiting for the database's own retention schedule (BackupRetain/
// BackupRetainDays, internal/backup.Scheduler) to age it out. Deletes the
// stored object behind the row first, best-effort (BackupDeleter's own
// doc comment: a storage-side failure there never blocks the row from
// being removed), then the store.BackupHistory row itself.
//
// AbilityWriteSensitive, matching handleTriggerBackup/handleSetBackupSchedule/
// handleDeleteBackupTarget: this destroys a real, previously-stored
// artifact, the same sensitivity class every other backup-mutating route
// in this file already carries. Not AbilityRoot: unlike
// handleTriggerRestore, nothing here touches a live database's actual
// data, only a historical archive of it.
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
		rt.logger.Error("api: delete backup failed", slog.String("error", err.Error()), slog.String("backup_id", historyID), slog.String("database", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteVolumeBackup handles
// DELETE /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}, the
// app service volume counterpart of handleDeleteBackup: identical
// behavior and AbilityWriteSensitive tier, reusing the exact same
// rt.backupDeleter (internal/backup.DeleteRunner is resource-agnostic,
// operating purely on a backup_history row's TargetID/ObjectKey/Status,
// the same reasoning handleDownloadVolumeBackup's own doc comment gives
// for rt.backupDownloader), only the ownership check and the URL differ.
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
		rt.logger.Error("api: delete volume backup failed", slog.String("error", err.Error()), slog.String("backup_id", historyID), slog.String("service", serviceName), slog.String("volume", volumeName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
