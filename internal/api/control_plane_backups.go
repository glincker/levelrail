package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
)

// ControlPlaneBackupManager is the surface the /system/backups routes need.
type ControlPlaneBackupManager interface {
	Create(ctx context.Context) (cpbackup.Info, error)
	List() ([]cpbackup.Info, error)
	Open(name string) (*os.File, cpbackup.Info, error)
	Delete(name string) error
}

// WithControlPlaneBackups enables the /api/v1/system/backups routes.
// Without one, they return 501.
func WithControlPlaneBackups(m ControlPlaneBackupManager) Option {
	return func(rt *Router) { rt.cpBackups = m }
}

// WithControlPlaneBackupScheduleDisabled tells the doctor check that
// scheduled snapshots are switched off, so it reports unknown, not a warning.
func WithControlPlaneBackupScheduleDisabled(disabled bool) Option {
	return func(rt *Router) { rt.cpBackupScheduleOff = disabled }
}

func (rt *Router) cpBackupsOrNotImplemented(w http.ResponseWriter) (ControlPlaneBackupManager, bool) {
	if rt.cpBackups == nil {
		writeError(w, http.StatusNotImplemented, "control plane backups are not configured")
		return nil, false
	}
	return rt.cpBackups, true
}

func (rt *Router) writeCPBackupError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, cpbackup.ErrInvalidName), errors.Is(err, cpbackup.ErrNotFound):
		writeError(w, http.StatusNotFound, "backup not found")
	default:
		rt.logger.Error("api: control plane backup failed", slog.String("action", action), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func (rt *Router) handleCreateControlPlaneBackup(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.cpBackupsOrNotImplemented(w)
	if !ok {
		return
	}
	info, err := m.Create(r.Context())
	if err != nil {
		rt.writeCPBackupError(w, "create", err)
		return
	}
	writeJSON(w, http.StatusCreated, info)
}

func (rt *Router) handleListControlPlaneBackups(w http.ResponseWriter, _ *http.Request) {
	m, ok := rt.cpBackupsOrNotImplemented(w)
	if !ok {
		return
	}
	list, err := m.List()
	if err != nil {
		rt.writeCPBackupError(w, "list", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (rt *Router) handleDownloadControlPlaneBackup(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.cpBackupsOrNotImplemented(w)
	if !ok {
		return
	}
	f, info, err := m.Open(r.PathValue("name"))
	if err != nil {
		rt.writeCPBackupError(w, "download", err)
		return
	}
	defer func() { _ = f.Close() }()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", info.Name))
	w.Header().Set("X-Content-SHA256", info.SHA256)
	if _, err := io.Copy(w, f); err != nil {
		rt.logger.Error("api: stream control plane backup failed", slog.String("error", err.Error()))
	}
}

func (rt *Router) handleDeleteControlPlaneBackup(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.cpBackupsOrNotImplemented(w)
	if !ok {
		return
	}
	if err := m.Delete(r.PathValue("name")); err != nil {
		rt.writeCPBackupError(w, "delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
