package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/dbprovision"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	provisionStatePending = "pending"
	provisionStateFailed  = "failed"
)

type pendingDatabaseLister interface {
	PendingDatabaseNames(ctx context.Context) (map[string]bool, error)
}

type existingVolumeSetter interface {
	SetDatabaseExistingVolume(ctx context.Context, name, choice string) error
}

// pendingDatabases lists databases with no container yet. A lookup error
// reports none pending rather than failing a read.
func (rt *Router) pendingDatabases(ctx context.Context) map[string]bool {
	l, ok := rt.databases.(pendingDatabaseLister)
	if !ok {
		return nil
	}
	names, err := l.PendingDatabaseNames(ctx)
	if err != nil {
		rt.logger.Warn("api: list pending databases failed", slog.String("error", err.Error()))
		return nil
	}
	return names
}

// withoutPendingDatabases drops databases that have no container yet, for
// callers that act on a running database (attach pickers, backup coverage).
func (rt *Router) withoutPendingDatabases(ctx context.Context, dbs []store.DesiredDatabase) []store.DesiredDatabase {
	return dropPendingDatabases(dbs, rt.pendingDatabases(ctx))
}

// dropPendingDatabases returns dbs without the names marked pending.
func dropPendingDatabases(dbs []store.DesiredDatabase, pending map[string]bool) []store.DesiredDatabase {
	if len(pending) == 0 {
		return dbs
	}
	out := make([]store.DesiredDatabase, 0, len(dbs))
	for _, d := range dbs {
		if !pending[d.Name] {
			out = append(out, d)
		}
	}
	return out
}

// stampProvisionState marks res as pending or failed while its newest
// provisioning attempt has not reached ready.
func (rt *Router) stampProvisionState(ctx context.Context, res *databaseResource) {
	a, found, err := rt.latestProvision(ctx, res.Name)
	if err != nil || !found || a.Status == dbprovision.AttemptReady {
		return
	}
	res.ProvisionState = provisionStatePending
	if a.Status == dbprovision.AttemptFailed && a.Failure != nil {
		res.ProvisionState = provisionStateFailed
		res.ProvisionReason = a.Failure.Reason
	}
}

// validateCreateBackup checks the backup settings sent with a create request
// without touching Docker, so everything is stored with the record.
func (rt *Router) validateCreateBackup(w http.ResponseWriter, r *http.Request, req databaseResource) bool {
	if req.BackupSchedule == "" && req.BackupTargetID == "" {
		return true
	}
	if refuseNoBackups(w, req.Engine) {
		return false
	}
	if !validateBackupScheduleRequest(w, req.BackupTargetID, req.BackupSchedule, req.BackupRetain, req.BackupRetainDays) {
		return false
	}
	return rt.loadBackupTarget(w, r, req.BackupTargetID, "api: create database: load backup target failed")
}

// pendingDatabaseOrWrite loads name and refuses with 409 once it has a
// running container, because only a never-created database may be edited or discarded.
func (rt *Router) pendingDatabaseOrWrite(w http.ResponseWriter, r *http.Request, name string) (*store.DesiredDatabase, bool) {
	d, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return nil, false
	}
	if err != nil {
		rt.internalError(w, "api: load pending database failed", err, slog.String("name", name))
		return nil, false
	}
	a, found, err := rt.latestProvision(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: load provisioning failed", err, slog.String("name", name))
		return nil, false
	}
	if !found || a.Status == dbprovision.AttemptReady {
		writeError(w, http.StatusConflict, "this database is already created; use its own settings instead")
		return nil, false
	}
	return d, true
}

type updatePendingDatabaseRequest struct {
	Version          string                  `json:"version"`
	Resources        *store.ServiceResources `json:"resources"`
	BackupTargetID   string                  `json:"backup_target_id"`
	BackupSchedule   string                  `json:"backup_schedule"`
	BackupRetain     int                     `json:"backup_retain"`
	BackupRetainDays int                     `json:"backup_retain_days"`
	ExistingVolume   string                  `json:"existing_volume"`
}

// handleUpdatePendingDatabase handles PUT /api/v1/databases/{name}/pending-settings:
// it replaces the stored settings of a database that never got a container.
func (rt *Router) handleUpdatePendingDatabase(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req updatePendingDatabaseRequest
	if !decodeReq(w, r, &req) {
		return
	}
	d, ok := rt.pendingDatabaseOrWrite(w, r, name)
	if !ok {
		return
	}
	next := *d
	if req.Version != "" {
		if err := database.ValidateEngineVersion(d.Engine, req.Version); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		next.Version = req.Version
	}
	choice, valid := validExistingVolume(req.ExistingVolume)
	if !valid {
		writeError(w, http.StatusBadRequest, "existing_volume must be \"reuse\" or \"discard\"")
		return
	}
	if !rt.validateCreateBackup(w, r, databaseResource{
		Engine: d.Engine, BackupTargetID: req.BackupTargetID, BackupSchedule: req.BackupSchedule,
		BackupRetain: req.BackupRetain, BackupRetainDays: req.BackupRetainDays,
	}) {
		return
	}
	next.Resources = req.Resources
	release, admitted := rt.admitDatabaseResizeQuota(w, r, *d, req.Resources)
	if !admitted {
		return
	}
	defer release()

	if err := rt.databases.SaveDesiredDatabase(r.Context(), next); err != nil {
		rt.internalError(w, "api: update pending database failed", err, slog.String("name", name))
		return
	}
	if err := rt.databases.SetDatabaseBackupSchedule(r.Context(), name, req.BackupTargetID, req.BackupSchedule, req.BackupRetain, req.BackupRetainDays); err != nil {
		rt.internalError(w, "api: update pending database: backup schedule failed", err, slog.String("name", name))
		return
	}
	if choice != "" {
		if s, ok := rt.databases.(existingVolumeSetter); ok {
			if err := s.SetDatabaseExistingVolume(r.Context(), name, choice); err != nil {
				rt.internalError(w, "api: update pending database: volume choice failed", err, slog.String("name", name))
				return
			}
		}
	}
	saved, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: update pending database: reload failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, rt.toDatabaseResourceWithStatus(r.Context(), *saved))
}

type discardPendingRequest struct {
	Confirm bool `json:"confirm"`
}

// handleDiscardPendingDatabase handles POST /api/v1/databases/{name}/provision/discard:
// it deletes the record of a database that never got a container. Data volumes are kept.
func (rt *Router) handleDiscardPendingDatabase(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req discardPendingRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if !req.Confirm {
		writeError(w, http.StatusBadRequest, "discarding deletes the saved settings: send confirm true to proceed")
		return
	}
	d, ok := rt.pendingDatabaseOrWrite(w, r, name)
	if !ok {
		return
	}
	if err := rt.databases.DeleteDesiredDatabase(r.Context(), name); err != nil && !errors.Is(err, store.ErrDatabaseNotFound) {
		rt.internalError(w, "api: discard pending database failed", err, slog.String("name", name))
		return
	}
	rt.teardownDatabaseContainer(name, d.NodeID)
	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}
