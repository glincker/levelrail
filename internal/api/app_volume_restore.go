package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ServiceVolumeRestoreHistoryStore is the store surface the volume
// restore history handler needs, the volume counterpart of
// RestoreHistoryStore (restore.go).
type ServiceVolumeRestoreHistoryStore interface {
	ListServiceVolumeRestoreHistory(ctx context.Context, serviceName, volumeName string) ([]store.RestoreHistory, error)
}

// ServiceVolumeRestoreRunner is the surface the volume restore trigger
// handler needs from internal/backup.RestoreRunner: the volume
// counterpart of RestoreRunner (restore.go). *backup.RestoreRunner
// satisfies this structurally, the same boundary RestoreRunner's own
// doc comment describes.
type ServiceVolumeRestoreRunner interface {
	RunVolumeRestore(ctx context.Context, historyID, serviceName, volumeName, dockerVolumeName, backupHistoryID string) error
}

type triggerVolumeRestoreRequest struct {
	BackupID string `json:"backup_id"`
}

// handleTriggerVolumeRestore handles
// POST /api/v1/apps/{name}/volumes/{volume}/restore, the volume
// counterpart of handleTriggerRestore (restore.go). AbilityRoot, the
// identical top tier that handler's own doc comment justifies at length:
// this is in-place, destructive, irreversible overwrite of a volume's
// entire contents, the exact same risk class as a database restore, not
// a lesser one just because the target is a filesystem instead of a
// schema.
func (rt *Router) handleTriggerVolumeRestore(w http.ResponseWriter, r *http.Request) {
	if rt.serviceVolumeRestoreRunner == nil {
		writeError(w, http.StatusNotImplemented, "restores are not configured on this control plane (no master key set)")
		return
	}

	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")

	dockerVolumeName, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: trigger volume restore: load service failed")
	if !ok {
		return
	}

	var req triggerVolumeRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.BackupID == "" {
		writeError(w, http.StatusBadRequest, "backup_id is required")
		return
	}

	backup, err := rt.backupHistory.GetBackupHistory(r.Context(), req.BackupID)
	if errors.Is(err, store.ErrBackupHistoryNotFound) {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: trigger volume restore: load backup history failed", slog.String("error", err.Error()), slog.String("backup_id", req.BackupID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if backup.ServiceName != serviceName || backup.VolumeName != volumeName {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("backup %q was not taken from %s/%s", req.BackupID, serviceName, volumeName))
		return
	}
	if backup.Status != store.BackupStatusSucceeded {
		writeError(w, http.StatusConflict, fmt.Sprintf("backup %q has status %q, only a succeeded backup can be restored from", req.BackupID, backup.Status))
		return
	}

	historyID, err := randomRestoreHistoryID()
	if err != nil {
		rt.logger.Error("api: trigger volume restore: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	go func() { //nolint:gosec // deliberately not r.Context(): it is cancelled the moment this handler returns, the same reasoning handleTriggerRestore's own goroutine gives
		if err := rt.serviceVolumeRestoreRunner.RunVolumeRestore(context.Background(), historyID, serviceName, volumeName, dockerVolumeName, req.BackupID); err != nil {
			rt.logger.Error("api: volume restore run failed", slog.String("error", err.Error()), slog.String("id", historyID), slog.String("service", serviceName), slog.String("volume", volumeName))
		}
	}()

	writeJSON(w, http.StatusAccepted, restoreHistoryResource{
		ID:              historyID,
		ServiceName:     serviceName,
		VolumeName:      volumeName,
		BackupHistoryID: req.BackupID,
		Status:          store.BackupStatusRunning,
	})
}

// handleListVolumeRestoreHistory handles
// GET /api/v1/apps/{name}/volumes/{volume}/restores, the volume
// counterpart of handleListRestoreHistory (restore.go).
func (rt *Router) handleListVolumeRestoreHistory(w http.ResponseWriter, r *http.Request) {
	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")

	if _, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: list volume restore history: load service failed"); !ok {
		return
	}

	history, err := rt.serviceVolumeRestoreHistory.ListServiceVolumeRestoreHistory(r.Context(), serviceName, volumeName)
	if err != nil {
		rt.logger.Error("api: list volume restore history failed", slog.String("error", err.Error()), slog.String("service", serviceName), slog.String("volume", volumeName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]restoreHistoryResource, 0, len(history))
	for _, h := range history {
		out = append(out, toRestoreHistoryResource(h))
	}
	writeJSON(w, http.StatusOK, out)
}
