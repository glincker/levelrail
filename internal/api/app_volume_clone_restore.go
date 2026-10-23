package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/ids"
)

// VolumeCloneRestoreHistoryStore is the store surface the volume
// clone-restore history handler needs, the app service volume counterpart
// of CloneRestoreHistoryStore (database_clone_restore.go).
type VolumeCloneRestoreHistoryStore interface {
	ListVolumeCloneRestores(ctx context.Context, serviceName, volumeName string) ([]store.VolumeCloneRestore, error)
}

// VolumeCloneRestoreRunner is the surface the volume clone-restore handler needs, satisfied by *backup.VolumeCloneRestoreRunner.
type VolumeCloneRestoreRunner interface {
	RunVolumeCloneRestore(ctx context.Context, historyID, sourceServiceName, sourceVolumeName, newVolumeName, backupHistoryID string) error
}

// volumeCloneRestoreResource is the wire shape for one "restore as new
// volume" attempt.
type volumeCloneRestoreResource struct {
	ID                string `json:"id"`
	SourceServiceName string `json:"source_service_name"`
	SourceVolumeName  string `json:"source_volume_name"`
	NewVolumeName     string `json:"new_volume_name"`
	BackupHistoryID   string `json:"backup_history_id"`
	Status            string `json:"status"`
	Error             string `json:"error,omitempty"`
	StartedAt         string `json:"started_at"`
	FinishedAt        string `json:"finished_at,omitempty"`
}

func toVolumeCloneRestoreResource(h store.VolumeCloneRestore) volumeCloneRestoreResource {
	return volumeCloneRestoreResource{
		ID:                h.ID,
		SourceServiceName: h.SourceServiceName,
		SourceVolumeName:  h.SourceVolumeName,
		NewVolumeName:     h.NewVolumeName,
		BackupHistoryID:   h.BackupHistoryID,
		Status:            h.Status,
		Error:             h.Error,
		StartedAt:         h.StartedAt,
		FinishedAt:        h.FinishedAt,
	}
}

// volumeCloneRestoreRequest is
// POST /api/v1/apps/{name}/volumes/{volume}/restore-as-new's body.
// NewVolumeName is optional: left blank, defaultVolumeCloneName mints one.
type volumeCloneRestoreRequest struct {
	BackupID      string `json:"backup_id"`
	NewVolumeName string `json:"new_volume_name,omitempty"`
}

// handleVolumeCloneRestore creates a new standalone Docker volume from a succeeded backup and never touches the live volume.
// It needs AbilityWriteSensitive, not AbilityRoot, because it never overwrites anything.
func (rt *Router) handleVolumeCloneRestore(w http.ResponseWriter, r *http.Request) {
	if rt.volumeCloneRestoreRunner == nil {
		writeError(w, http.StatusNotImplemented, "restores are not configured on this control plane (no master key set)")
		return
	}

	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")

	if _, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: volume clone restore: load service failed"); !ok {
		return
	}

	var req volumeCloneRestoreRequest
	if !decodeReq(w, r, &req) {
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
		rt.internalError(w, "api: volume clone restore: load backup history failed", err, slog.String("backup_id", req.BackupID))
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

	newVolumeName := strings.TrimSpace(req.NewVolumeName)
	if newVolumeName == "" {
		newVolumeName, err = defaultVolumeCloneName(serviceName, volumeName)
		if err != nil {
			rt.internalError(w, "api: volume clone restore: generate default volume name failed", err)
			return
		}
	}

	historyID, err := randomVolumeCloneRestoreID()
	if err != nil {
		rt.internalError(w, "api: volume clone restore: generate id failed", err)
		return
	}

	go func() { //nolint:gosec // deliberately not r.Context(): it is cancelled the moment this handler returns, the same reasoning handleCloneRestore's own goroutine gives
		if err := rt.volumeCloneRestoreRunner.RunVolumeCloneRestore(context.Background(), historyID, serviceName, volumeName, newVolumeName, req.BackupID); err != nil {
			rt.logger.Error("api: volume clone restore run failed", slog.String("error", err.Error()), slog.String("id", historyID), slog.String("service", serviceName), slog.String("volume", volumeName))
		}
	}()

	writeJSON(w, http.StatusAccepted, volumeCloneRestoreResource{
		ID:                historyID,
		SourceServiceName: serviceName,
		SourceVolumeName:  volumeName,
		NewVolumeName:     newVolumeName,
		BackupHistoryID:   req.BackupID,
		Status:            store.BackupStatusRunning,
	})
}

func (rt *Router) handleListVolumeCloneRestores(w http.ResponseWriter, r *http.Request) {
	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")

	if _, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: list volume clone restores: load service failed"); !ok {
		return
	}

	history, err := rt.volumeCloneRestoreHistory.ListVolumeCloneRestores(r.Context(), serviceName, volumeName)
	if err != nil {
		rt.internalError(w, "api: list volume clone restores failed", err, slog.String("service", serviceName), slog.String("volume", volumeName))
		return
	}
	out := make([]volumeCloneRestoreResource, 0, len(history))
	for _, h := range history {
		out = append(out, toVolumeCloneRestoreResource(h))
	}
	writeJSON(w, http.StatusOK, out)
}

// Embeds the source service and volume so the lineage shows in docker volume ls, plus a random suffix to avoid collisions.
func defaultVolumeCloneName(serviceName, volumeName string) (string, error) {
	suffix, err := randomHexSuffix()
	if err != nil {
		return "", fmt.Errorf("generate default volume name: %w", err)
	}
	return "clone-" + serviceName + "-" + volumeName + "-" + suffix, nil
}

func randomHexSuffix() (string, error) {
	return ids.Hex(6)
}

// The vcr_ prefix keeps these IDs distinct from database clone-restore and restore-history IDs in logs.
func randomVolumeCloneRestoreID() (string, error) {
	s, err := ids.URL(9)
	if err != nil {
		return "", fmt.Errorf("api: generate volume clone restore id: %w", err)
	}
	return "vcr_" + s, nil
}
