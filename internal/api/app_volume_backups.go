package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/cronexpr"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ServiceVolumeBackupHistoryStore is the store surface the volume backup
// history handler needs, the volume counterpart of BackupHistoryStore
// (backups.go): GetBackupHistory is already generic by ID (shared with
// the database path via rt.backupHistory), so only the listing method
// needs its own volume-scoped query.
type ServiceVolumeBackupHistoryStore interface {
	ListServiceVolumeBackupHistory(ctx context.Context, serviceName, volumeName string, limit int, before *time.Time) ([]store.BackupHistory, error)
}

// ServiceVolumeBackupRunner is the surface the volume backup trigger
// handler needs from internal/backup.Runner: the volume counterpart of
// BackupRunner (backups.go). *backup.Runner satisfies this
// structurally, the same boundary BackupRunner's own doc comment
// describes.
type ServiceVolumeBackupRunner interface {
	RunVolumeBackup(ctx context.Context, historyID, serviceName, volumeName, dockerVolumeName, targetID string) error
}

// ServiceVolumeBackupScheduleStore is the store surface the volume
// backup schedule handlers need: the volume counterpart of
// SetDatabaseBackupSchedule/GetDatabaseBackupSchedule-shaped access, kept
// as its own table (service_volume_backups) rather than columns on
// desired_services, see migrations/0075's own doc comment.
type ServiceVolumeBackupScheduleStore interface {
	SetServiceVolumeBackupSchedule(ctx context.Context, serviceName, volumeName, targetID, schedule string, retain, retainDays int) error
	GetServiceVolumeBackupSchedule(ctx context.Context, serviceName, volumeName string) (store.ServiceVolumeBackupConfig, error)
}

type triggerVolumeBackupRequest struct {
	TargetID string `json:"target_id"`
}

// handleTriggerVolumeBackup handles
// POST /api/v1/apps/{name}/volumes/{volume}/backups: the volume
// counterpart of handleTriggerBackup (backups.go), same "starts real
// work, returns once it's recorded and under way" shape, same
// AbilityWriteSensitive tier.
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.TargetID == "" {
		writeError(w, http.StatusBadRequest, "target_id is required")
		return
	}
	if !rt.loadBackupTarget(w, r, req.TargetID, "api: trigger volume backup: load backup target failed") {
		return
	}

	historyID, err := randomBackupHistoryID()
	if err != nil {
		rt.logger.Error("api: trigger volume backup: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	go func() { //nolint:gosec // deliberately not r.Context(): it is cancelled the moment this handler returns, the same reasoning handleTriggerBackup's own goroutine gives
		if err := rt.serviceVolumeBackupRunner.RunVolumeBackup(context.Background(), historyID, serviceName, volumeName, dockerVolumeName, req.TargetID); err != nil {
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

	limit := defaultBackupHistoryLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = n
	}
	if limit > maxBackupHistoryLimit {
		limit = maxBackupHistoryLimit
	}

	var before *time.Time
	if raw := r.URL.Query().Get("before"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "before must be an RFC3339 timestamp")
			return
		}
		before = &t
	}

	history, err := rt.serviceVolumeBackupHistory.ListServiceVolumeBackupHistory(r.Context(), serviceName, volumeName, limit, before)
	if err != nil {
		rt.logger.Error("api: list volume backup history failed", slog.String("error", err.Error()), slog.String("service", serviceName), slog.String("volume", volumeName))
		writeError(w, http.StatusInternalServerError, "internal error")
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
}

// handleGetVolumeBackupSchedule handles
// GET /api/v1/apps/{name}/volumes/{volume}/backup-schedule. Unlike the
// database path (whose schedule fields ride along on GET .../databases/
// {name} instead of a dedicated route), a service can have any number of
// volumes, so there is no single app resource response to embed a
// volume's own schedule into; this is that dedicated GET.
// ErrServiceVolumeBackupNotFound is not an error response, it's the
// normal "no schedule configured" state, mirroring an empty
// backup_schedule column's meaning on the database side.
func (rt *Router) handleGetVolumeBackupSchedule(w http.ResponseWriter, r *http.Request) {
	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")

	if _, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: get volume backup schedule: load service failed"); !ok {
		return
	}

	cfg, err := rt.serviceVolumeBackupSchedule.GetServiceVolumeBackupSchedule(r.Context(), serviceName, volumeName)
	if err != nil && !errors.Is(err, store.ErrServiceVolumeBackupNotFound) {
		rt.logger.Error("api: get volume backup schedule failed", slog.String("error", err.Error()), slog.String("service", serviceName), slog.String("volume", volumeName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, volumeBackupScheduleResource{
		ServiceName: serviceName,
		VolumeName:  volumeName,
		TargetID:    cfg.BackupTargetID,
		Schedule:    cfg.BackupSchedule,
		Retain:      cfg.BackupRetain,
		RetainDays:  cfg.BackupRetainDays,
	})
}

type setVolumeBackupScheduleRequest struct {
	TargetID   string `json:"target_id"`
	Schedule   string `json:"schedule"`
	Retain     int    `json:"retain,omitempty"`
	RetainDays int    `json:"retain_days,omitempty"`
}

// handleSetVolumeBackupSchedule handles
// PUT /api/v1/apps/{name}/volumes/{volume}/backup-schedule, the volume
// counterpart of handleSetBackupSchedule (backups.go): identical
// synchronous validation (cron string, non-negative retention) before
// ever writing state.
func (rt *Router) handleSetVolumeBackupSchedule(w http.ResponseWriter, r *http.Request) {
	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")

	if _, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: set volume backup schedule: load service failed"); !ok {
		return
	}

	var req setVolumeBackupScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.TargetID == "" {
		writeError(w, http.StatusBadRequest, "target_id is required")
		return
	}
	if req.Schedule == "" {
		writeError(w, http.StatusBadRequest, "schedule is required")
		return
	}
	if _, err := cronexpr.Parse(req.Schedule); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid schedule: %s", err.Error()))
		return
	}
	if req.Retain < 0 {
		writeError(w, http.StatusBadRequest, "retain must not be negative")
		return
	}
	if req.RetainDays < 0 {
		writeError(w, http.StatusBadRequest, "retain_days must not be negative")
		return
	}
	if !rt.loadBackupTarget(w, r, req.TargetID, "api: set volume backup schedule: load backup target failed") {
		return
	}

	if err := rt.serviceVolumeBackupSchedule.SetServiceVolumeBackupSchedule(r.Context(), serviceName, volumeName, req.TargetID, req.Schedule, req.Retain, req.RetainDays); err != nil {
		rt.logger.Error("api: set volume backup schedule failed", slog.String("error", err.Error()), slog.String("service", serviceName), slog.String("volume", volumeName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, volumeBackupScheduleResource{
		ServiceName: serviceName,
		VolumeName:  volumeName,
		TargetID:    req.TargetID,
		Schedule:    req.Schedule,
		Retain:      req.Retain,
		RetainDays:  req.RetainDays,
	})
}

// handleClearVolumeBackupSchedule handles
// DELETE /api/v1/apps/{name}/volumes/{volume}/backup-schedule, the
// reverse of handleSetVolumeBackupSchedule, mirroring
// handleClearBackupSchedule (backups.go).
func (rt *Router) handleClearVolumeBackupSchedule(w http.ResponseWriter, r *http.Request) {
	serviceName := r.PathValue("name")
	volumeName := r.PathValue("volume")

	if _, ok := rt.loadServiceVolume(w, r, serviceName, volumeName, "api: clear volume backup schedule: load service failed"); !ok {
		return
	}

	if err := rt.serviceVolumeBackupSchedule.SetServiceVolumeBackupSchedule(r.Context(), serviceName, volumeName, "", "", 0, 0); err != nil {
		rt.logger.Error("api: clear volume backup schedule failed", slog.String("error", err.Error()), slog.String("service", serviceName), slog.String("volume", volumeName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
