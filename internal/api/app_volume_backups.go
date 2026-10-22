package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ServiceVolumeBackupHistoryStore is the store surface the volume backup history handler needs.
type ServiceVolumeBackupHistoryStore interface {
	ListServiceVolumeBackupHistory(ctx context.Context, serviceName, volumeName string, limit int, before *time.Time) ([]store.BackupHistory, error)
}

// ServiceVolumeBackupRunner is the surface the volume backup trigger needs from backup.Runner.
type ServiceVolumeBackupRunner interface {
	RunVolumeBackup(ctx context.Context, historyID, serviceName, volumeName, dockerVolumeName, targetID, sqlitePath string) error
}

// ServiceVolumeBackupScheduleStore is the store surface for volume backup schedules, kept in service_volume_backups.
type ServiceVolumeBackupScheduleStore interface {
	SetServiceVolumeBackupSchedule(ctx context.Context, serviceName, volumeName, targetID, schedule string, retain, retainDays int, sqlitePath string) error
	GetServiceVolumeBackupSchedule(ctx context.Context, serviceName, volumeName string) (store.ServiceVolumeBackupConfig, error)
}

type triggerVolumeBackupRequest struct {
	TargetID string `json:"target_id"`
}

// handleTriggerVolumeBackup starts a volume backup and returns once it is recorded and under way.
func (rt *Router) handleTriggerVolumeBackup(w http.ResponseWriter, r *http.Request) {
	if rt.serviceVolumeBackupRunner == nil {
		writeError(w, http.StatusNotImplemented, "backups are not configured on this control plane (no master key set)")
		return
	}

	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")

	dockerVolumeName, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: trigger volume backup: load service failed")
	if !ok {
		return
	}

	var req triggerVolumeBackupRequest
	if !decodeReq(w, r, &req) {
		return
	}

	historyID, ok := rt.prepareBackupTrigger(w, r, req.TargetID, "api: trigger volume backup")
	if !ok {
		return
	}

	cfg, err := rt.serviceVolumeBackupSchedule.GetServiceVolumeBackupSchedule(r.Context(), serviceName, volumeName)
	if err != nil && !errors.Is(err, store.ErrServiceVolumeBackupNotFound) {
		rt.internalError(w, "api: trigger volume backup: load sqlite path failed", err)
		return
	}

	go func() { //nolint:gosec // deliberately not r.Context(): it is cancelled the moment this handler returns, the same reasoning handleTriggerBackup's own goroutine gives
		if err := rt.serviceVolumeBackupRunner.RunVolumeBackup(context.Background(), historyID, serviceName, volumeName, dockerVolumeName, req.TargetID, cfg.SqlitePath); err != nil {
			rt.logger.Error("api: volume backup run failed", slog.String("error", err.Error()), slog.String("id", historyID), slog.String("service", serviceName), slog.String("volume", volumeName))
		}
	}()

	writeJSON(w, http.StatusAccepted, backupHistoryResource{
		ID:          historyID,
		ServiceName: serviceName,
		VolumeName:  volumeName,
		TargetID:    req.TargetID,
		Status:      store.BackupStatusRunning,
	})
}

// handleListVolumeBackupHistory handles
// GET /api/v1/apps/{name}/volumes/{volume}/backups, the volume
// counterpart of handleListBackupHistory (backups.go).
func (rt *Router) handleListVolumeBackupHistory(w http.ResponseWriter, r *http.Request) {
	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")

	if _, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: list volume backup history: load service failed"); !ok {
		return
	}

	limit, before, ok := parseBackupHistoryListParams(w, r)
	if !ok {
		return
	}

	history, err := rt.serviceVolumeBackupHistory.ListServiceVolumeBackupHistory(r.Context(), serviceName, volumeName, limit, before)
	if err != nil {
		rt.internalError(w, "api: list volume backup history failed", err, slog.String("service", serviceName), slog.String("volume", volumeName))
		return
	}
	out := make([]backupHistoryResource, 0, len(history))
	for _, h := range history {
		out = append(out, toBackupHistoryResource(h))
	}
	writeJSON(w, http.StatusOK, out)
}

// volumeBackupScheduleResource is the wire shape for a service volume's
// scheduled backup config, the volume counterpart of
// backupScheduleResource (backups.go).
type volumeBackupScheduleResource struct {
	ServiceName string `json:"service_name"`
	VolumeName  string `json:"volume_name"`
	TargetID    string `json:"target_id,omitempty"`
	Schedule    string `json:"schedule,omitempty"`
	Retain      int    `json:"retain,omitempty"`
	RetainDays  int    `json:"retain_days,omitempty"`
	SqlitePath  string `json:"sqlite_path,omitempty"`
}

// handleGetVolumeBackupSchedule handles GET .../backup-schedule. A missing schedule is a normal state, not an error.
func (rt *Router) handleGetVolumeBackupSchedule(w http.ResponseWriter, r *http.Request) {
	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")

	if _, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: get volume backup schedule: load service failed"); !ok {
		return
	}

	cfg, err := rt.serviceVolumeBackupSchedule.GetServiceVolumeBackupSchedule(r.Context(), serviceName, volumeName)
	if err != nil && !errors.Is(err, store.ErrServiceVolumeBackupNotFound) {
		rt.internalError(w, "api: get volume backup schedule failed", err, slog.String("service", serviceName), slog.String("volume", volumeName))
		return
	}

	writeJSON(w, http.StatusOK, volumeBackupScheduleResource{
		ServiceName: serviceName,
		VolumeName:  volumeName,
		TargetID:    cfg.BackupTargetID,
		Schedule:    cfg.BackupSchedule,
		Retain:      cfg.BackupRetain,
		RetainDays:  cfg.BackupRetainDays,
		SqlitePath:  cfg.SqlitePath,
	})
}

type setVolumeBackupScheduleRequest struct {
	TargetID   string `json:"target_id"`
	Schedule   string `json:"schedule"`
	Retain     int    `json:"retain,omitempty"`
	RetainDays int    `json:"retain_days,omitempty"`
	SqlitePath string `json:"sqlite_path,omitempty"`
}

// handleSetVolumeBackupSchedule validates the cron and retention synchronously before writing state.
func (rt *Router) handleSetVolumeBackupSchedule(w http.ResponseWriter, r *http.Request) {
	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")

	if _, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: set volume backup schedule: load service failed"); !ok {
		return
	}

	var req setVolumeBackupScheduleRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if !validateBackupScheduleRequest(w, req.TargetID, req.Schedule, req.Retain, req.RetainDays) {
		return
	}
	if req.SqlitePath != "" {
		if err := backup.ValidateSqlitePath(req.SqlitePath); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if !rt.loadBackupTarget(w, r, req.TargetID, "api: set volume backup schedule: load backup target failed") {
		return
	}

	if err := rt.serviceVolumeBackupSchedule.SetServiceVolumeBackupSchedule(r.Context(), serviceName, volumeName, req.TargetID, req.Schedule, req.Retain, req.RetainDays, req.SqlitePath); err != nil {
		rt.internalError(w, "api: set volume backup schedule failed", err, slog.String("service", serviceName), slog.String("volume", volumeName))
		return
	}

	writeJSON(w, http.StatusOK, volumeBackupScheduleResource{
		ServiceName: serviceName,
		VolumeName:  volumeName,
		TargetID:    req.TargetID,
		Schedule:    req.Schedule,
		Retain:      req.Retain,
		RetainDays:  req.RetainDays,
		SqlitePath:  req.SqlitePath,
	})
}

func (rt *Router) handleClearVolumeBackupSchedule(w http.ResponseWriter, r *http.Request) {
	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")

	if _, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: clear volume backup schedule: load service failed"); !ok {
		return
	}

	if err := rt.serviceVolumeBackupSchedule.SetServiceVolumeBackupSchedule(r.Context(), serviceName, volumeName, "", "", 0, 0, ""); err != nil {
		rt.internalError(w, "api: clear volume backup schedule failed", err, slog.String("service", serviceName), slog.String("volume", volumeName))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// loadVolumeBackupHistory resolves historyID and checks it came from this service and volume, writing the error response on failure.
func (rt *Router) loadVolumeBackupHistory(w http.ResponseWriter, r *http.Request, serviceName, volumeName, historyID, logContext string) (store.BackupHistory, bool) {
	h, err := rt.backupHistory.GetBackupHistory(r.Context(), historyID)
	if errors.Is(err, store.ErrBackupHistoryNotFound) {
		writeError(w, http.StatusNotFound, "backup not found")
		return store.BackupHistory{}, false
	}
	if err != nil {
		rt.internalError(w, logContext, err, slog.String("backup_id", historyID))
		return store.BackupHistory{}, false
	}
	if h.ServiceName != serviceName || h.VolumeName != volumeName {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("backup %q was not taken from %s/%s", historyID, serviceName, volumeName))
		return store.BackupHistory{}, false
	}
	return h, true
}
