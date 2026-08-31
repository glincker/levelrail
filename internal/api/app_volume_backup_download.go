package api

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// handleDownloadVolumeBackup handles
// GET /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/download,
// the volume counterpart of handleDownloadBackup (backup_download.go):
// identical streaming behavior and AbilityReadSensitive tier, reusing
// the exact same rt.backupDownloader (internal/backup.DownloadRunner is
// already resource-agnostic, operating purely on a backup_history row's
// TargetID/ObjectKey/Status), only the ownership check and the URL
// differ.
func (rt *Router) handleDownloadVolumeBackup(w http.ResponseWriter, r *http.Request) {
	if rt.backupDownloader == nil {
		writeError(w, http.StatusNotImplemented, "backup downloads are not configured on this control plane (no master key set)")
		return
	}

	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")
	historyID := r.PathValue("historyId")

	if _, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: download volume backup: load service failed"); !ok {
		return
	}

	h, err := rt.backupHistory.GetBackupHistory(r.Context(), historyID)
	if errors.Is(err, store.ErrBackupHistoryNotFound) {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: download volume backup: load backup history failed", slog.String("error", err.Error()), slog.String("backup_id", historyID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if h.ServiceName != serviceName || h.VolumeName != volumeName {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("backup %q was not taken from %s/%s", historyID, serviceName, volumeName))
		return
	}
	if h.Status != store.BackupStatusSucceeded {
		writeError(w, http.StatusConflict, fmt.Sprintf("backup %q has status %q, only a succeeded backup can be downloaded", historyID, h.Status))
		return
	}

	stream, err := rt.backupDownloader.Download(r.Context(), historyID)
	if err != nil {
		rt.logger.Error("api: download volume backup failed", slog.String("error", err.Error()), slog.String("backup_id", historyID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer func() {
		_ = stream.Close()
	}()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", volumeDownloadFilename(h)))
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, stream); err != nil {
		rt.logger.Error("api: download volume backup: stream failed", slog.String("error", err.Error()), slog.String("backup_id", historyID))
	}
}

// volumeDownloadFilename mirrors downloadFilename's own reasoning
// (backup_download.go), falling back to service/volume plus timestamp
// instead of database name.
func volumeDownloadFilename(h store.BackupHistory) string {
	if base := path.Base(h.ObjectKey); base != "" && base != "." && base != "/" {
		return base
	}
	return fmt.Sprintf("%s-%s-%s.tar", h.ServiceName, h.VolumeName, strings.ReplaceAll(h.StartedAt, ":", ""))
}
