package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// handleTriggerMySQLPITRRestore restores a MySQL or MariaDB database to a point
// in time by creating a new database and replaying the binlog into it. The
// source is never written to, so unlike Postgres this is not root-destructive.
func (rt *Router) handleTriggerMySQLPITRRestore(w http.ResponseWriter, r *http.Request, db *store.DesiredDatabase) {
	if rt.mysqlPITRRestoreRunner == nil {
		writeError(w, http.StatusNotImplemented, "restores are not configured on this control plane (no master key set)")
		return
	}
	name := db.Name
	if !db.PITREnabled {
		writeError(w, http.StatusConflict, "point-in-time restore is not enabled for this database")
		return
	}

	var req triggerPITRRestoreRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if req.BaseBackupID == "" {
		writeError(w, http.StatusBadRequest, "base_backup_id is required")
		return
	}
	if req.RestoreAs == "" {
		writeError(w, http.StatusBadRequest, "restore_as is required: a point-in-time restore of "+db.Engine+" creates a new database")
		return
	}
	if !validDatabaseName(req.RestoreAs) {
		writeError(w, http.StatusBadRequest, "restore_as must be 1-63 characters: lowercase letters, digits and hyphens, starting with a letter")
		return
	}
	if req.RestoreAs == name {
		writeError(w, http.StatusBadRequest, "restore_as must differ from the source database's own name")
		return
	}
	target, err := time.Parse(time.RFC3339, req.TargetTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "target_time must be an RFC3339 timestamp")
		return
	}

	sourceContainer := databaseContainerName(name)
	if err := rt.mysqlPITRRestoreRunner.ValidateTarget(r.Context(), name, sourceContainer, target); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	historyID, err := randomPITRRestoreHistoryID()
	if err != nil {
		rt.internalError(w, "api: trigger pitr restore: generate id failed", err)
		return
	}

	if !rt.createDesiredDatabase(w, r, databaseResource{Name: req.RestoreAs, Engine: db.Engine, Version: db.Version, ProjectID: db.ProjectID}) {
		return
	}

	rt.nudgeReconciler()
	newContainer := databaseContainerName(req.RestoreAs)
	newController := databaseControllerName(req.RestoreAs)
	go func() { //nolint:gosec // deliberately not r.Context(): it is cancelled when this handler returns, which would abort the restore at once
		if err := rt.mysqlPITRRestoreRunner.RunPITRCloneRestore(context.Background(), historyID, name, req.RestoreAs, req.BaseBackupID, db.Engine, sourceContainer, newContainer, newController, target); err != nil {
			rt.logger.Error("api: pitr restore run failed", slog.String("error", err.Error()), slog.String("id", historyID), slog.String("database", name), slog.String("new_database", req.RestoreAs))
		}
	}()

	writeJSON(w, http.StatusAccepted, pitrRestoreHistoryResource{
		ID:                  historyID,
		DatabaseName:        name,
		BaseBackupHistoryID: req.BaseBackupID,
		TargetTimestamp:     target.UTC().Format(time.RFC3339),
		Status:              store.BackupStatusRunning,
	})
}
