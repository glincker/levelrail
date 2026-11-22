package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// backupPinStore is optional on the history store, so test fakes need not implement it.
type backupPinStore interface {
	SetBackupPinned(ctx context.Context, historyID string, pinned bool) error
	PinnedBackups(ctx context.Context, ids []string) (map[string]bool, error)
}

// attachBackupPins fills Pinned on resources; a read failure leaves them unpinned rather than failing the list.
func (rt *Router) attachBackupPins(ctx context.Context, out []backupHistoryResource) {
	ps, ok := rt.backupHistory.(backupPinStore)
	if !ok || len(out) == 0 {
		return
	}
	ids := make([]string, len(out))
	for i := range out {
		ids[i] = out[i].ID
	}
	pins, err := ps.PinnedBackups(ctx, ids)
	if err != nil {
		rt.logger.Warn("api: read backup pins failed", slog.String("error", err.Error()))
		return
	}
	for i := range out {
		out[i].Pinned = pins[out[i].ID]
	}
}

// setBackupPinned is the shared tail of the pin and unpin routes once the backup's ownership is checked.
func (rt *Router) setBackupPinned(w http.ResponseWriter, r *http.Request, h store.BackupHistory, pinned bool) {
	ps, ok := rt.backupHistory.(backupPinStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "backup pins are not available on this control plane")
		return
	}
	if pinned && refuseDeletingRunningBackup(w, h.ID, h.Status) {
		return
	}
	if err := ps.SetBackupPinned(r.Context(), h.ID, pinned); err != nil {
		if errors.Is(err, store.ErrBackupHistoryNotFound) {
			writeError(w, http.StatusNotFound, "backup not found")
			return
		}
		rt.internalError(w, "api: set backup pin failed", err, slog.String("backup_id", h.ID))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePinBackup handles PUT /api/v1/databases/{name}/backups/{historyId}/pin: keep this backup forever.
// Retention, pruning and delete all skip a pinned backup until it is unpinned.
func (rt *Router) handlePinBackup(w http.ResponseWriter, r *http.Request) {
	h, ok := rt.loadDatabaseBackupHistory(w, r, r.PathValue("name"), r.PathValue("historyId"), "api: pin backup: load backup history failed")
	if !ok {
		return
	}
	rt.setBackupPinned(w, r, h, true)
}

// handleUnpinBackup handles DELETE /api/v1/databases/{name}/backups/{historyId}/pin.
func (rt *Router) handleUnpinBackup(w http.ResponseWriter, r *http.Request) {
	h, ok := rt.loadDatabaseBackupHistory(w, r, r.PathValue("name"), r.PathValue("historyId"), "api: unpin backup: load backup history failed")
	if !ok {
		return
	}
	rt.setBackupPinned(w, r, h, false)
}

// handlePinVolumeBackup is the app-volume counterpart of handlePinBackup.
func (rt *Router) handlePinVolumeBackup(w http.ResponseWriter, r *http.Request) {
	rt.pinVolumeBackup(w, r, true)
}

// handleUnpinVolumeBackup is the app-volume counterpart of handleUnpinBackup.
func (rt *Router) handleUnpinVolumeBackup(w http.ResponseWriter, r *http.Request) {
	rt.pinVolumeBackup(w, r, false)
}

func (rt *Router) pinVolumeBackup(w http.ResponseWriter, r *http.Request, pinned bool) {
	serviceName, volumeName := r.PathValue("name"), r.PathValue("volume")
	if _, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: pin volume backup: load service failed"); !ok {
		return
	}
	h, ok := rt.loadVolumeBackupHistory(w, r, serviceName, volumeName, r.PathValue("historyId"), "api: pin volume backup: load backup history failed")
	if !ok {
		return
	}
	rt.setBackupPinned(w, r, h, pinned)
}
