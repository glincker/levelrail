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

// databaseBranchRequest is POST /api/v1/databases/{name}/branches. Exactly
// one of BackupID or At picks the data the branch starts from.
type databaseBranchRequest struct {
	Name      string `json:"name"`
	BackupID  string `json:"backup_id,omitempty"`
	At        string `json:"at,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

type databaseBranchResource struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	BranchedFrom string `json:"branched_from"`
	BackupID     string `json:"backup_id,omitempty"`
	At           string `json:"at,omitempty"`
	Status       string `json:"status"`
}

// handleBranchDatabase creates a new independent database from a backup, or from
// a point in time on engines whose PITR restore lands in a new database. It reuses
// the restore-as-new runners, never writes to the source, and never touches its
// secrets: the new database gets its own credentials when it is provisioned.
// AbilityWriteSensitive like restore-as-new, since it spends a stored backup credential.
func (rt *Router) handleBranchDatabase(w http.ResponseWriter, r *http.Request) {
	source := r.PathValue("name")

	sourceDB, ok := rt.loadDatabaseForRunner(w, r, source, "api: branch database: load source failed")
	if !ok || refuseNoBackups(w, sourceDB.Engine) {
		return
	}

	var req databaseBranchRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if !validDatabaseName(req.Name) {
		writeError(w, http.StatusBadRequest, "name must be 1-63 characters: lowercase letters, digits and hyphens, starting with a letter")
		return
	}
	if req.Name == source {
		writeError(w, http.StatusBadRequest, "name must differ from the source database's own name")
		return
	}
	if (req.BackupID == "") == (req.At == "") {
		writeError(w, http.StatusBadRequest, "give exactly one of backup_id or at")
		return
	}
	if _, err := rt.databases.GetDesiredDatabase(r.Context(), req.Name); err == nil {
		writeError(w, http.StatusConflict, "a database with this name already exists")
		return
	} else if !errors.Is(err, store.ErrDatabaseNotFound) {
		rt.internalError(w, "api: branch database: check name failed", err, slog.String("name", req.Name))
		return
	}

	var start func(historyID string)
	var out databaseBranchResource
	if req.BackupID != "" {
		if !rt.prepareBranchFromBackup(w, r, sourceDB, req) {
			return
		}
		start = func(historyID string) {
			rt.runBranchRestore(func(ctx context.Context) error {
				return rt.cloneRestoreRunner.RunCloneRestore(ctx, historyID, source, req.Name, req.BackupID, sourceDB.Engine, databaseContainerName(req.Name), databaseControllerName(req.Name))
			}, historyID, source, req.Name)
		}
		out = databaseBranchResource{BackupID: req.BackupID}
	} else {
		target, baseID, ok := rt.prepareBranchAtTime(w, r, sourceDB, req)
		if !ok {
			return
		}
		start = func(historyID string) {
			rt.runBranchRestore(func(ctx context.Context) error {
				return rt.mysqlPITRRestoreRunner.RunPITRCloneRestore(ctx, historyID, source, req.Name, baseID, sourceDB.Engine, databaseContainerName(source), databaseContainerName(req.Name), databaseControllerName(req.Name), target)
			}, historyID, source, req.Name)
		}
		out = databaseBranchResource{BackupID: baseID, At: target.UTC().Format(time.RFC3339)}
	}

	if err := backup.CheckWorkDirSpace(); err != nil {
		writeError(w, http.StatusInsufficientStorage, err.Error())
		return
	}
	historyID, err := randomCloneRestoreID()
	if err != nil {
		rt.internalError(w, "api: branch database: generate id failed", err)
		return
	}

	// Same engine and version as the source; the project follows the source unless overridden.
	projectID := req.ProjectID
	if projectID == "" {
		projectID = sourceDB.ProjectID
	}
	if !rt.createDesiredDatabase(w, r, databaseResource{Name: req.Name, Engine: sourceDB.Engine, Version: sourceDB.Version, ProjectID: projectID, Resources: copyResources(sourceDB.Resources)}) {
		return
	}
	if !rt.finishBranchSetup(w, r, sourceDB, req.Name) {
		return
	}
	rt.nudgeReconciler()
	start(historyID)

	out.ID, out.Name, out.BranchedFrom, out.Status = historyID, req.Name, source, store.BackupStatusRunning
	writeJSON(w, http.StatusAccepted, out)
}

func (rt *Router) runBranchRestore(run func(ctx context.Context) error, historyID, source, name string) {
	go func() { //nolint:gosec // deliberately not r.Context(): it is cancelled when the handler returns, which would abort the restore at once
		if err := run(context.Background()); err != nil {
			rt.logger.Error("api: database branch run failed", slog.String("error", err.Error()), slog.String("id", historyID), slog.String("source_database", source), slog.String("new_database", name))
		}
	}()
}

func (rt *Router) prepareBranchFromBackup(w http.ResponseWriter, r *http.Request, sourceDB *store.DesiredDatabase, req databaseBranchRequest) bool {
	if rt.cloneRestoreRunner == nil {
		writeError(w, http.StatusNotImplemented, "restores are not configured on this control plane (no master key set)")
		return false
	}
	b, err := rt.backupHistory.GetBackupHistory(r.Context(), req.BackupID)
	if errors.Is(err, store.ErrBackupHistoryNotFound) {
		writeError(w, http.StatusNotFound, "backup not found")
		return false
	}
	if err != nil {
		rt.internalError(w, "api: branch database: load backup failed", err, slog.String("backup_id", req.BackupID))
		return false
	}
	if b.DatabaseName != sourceDB.Name {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("backup %q was taken from database %q, not %q", req.BackupID, b.DatabaseName, sourceDB.Name))
		return false
	}
	if b.Status != store.BackupStatusSucceeded {
		writeError(w, http.StatusConflict, fmt.Sprintf("backup %q has status %q, only a succeeded backup can be branched from", req.BackupID, b.Status))
		return false
	}
	if refuseForeignBackup(w, sourceDB, b.ID, b.InstanceID, b.StartedAt) {
		return false
	}
	return true
}

// prepareBranchAtTime picks the newest base backup taken at or before the target.
// Only engines whose PITR restore creates a new database can branch this way.
func (rt *Router) prepareBranchAtTime(w http.ResponseWriter, r *http.Request, sourceDB *store.DesiredDatabase, req databaseBranchRequest) (time.Time, string, bool) {
	if !isMySQLFamilyEngine(sourceDB.Engine) {
		writeError(w, http.StatusConflict, "branching to a point in time is not available for "+sourceDB.Engine+" databases, use backup_id")
		return time.Time{}, "", false
	}
	if rt.mysqlPITRRestoreRunner == nil {
		writeError(w, http.StatusNotImplemented, "restores are not configured on this control plane (no master key set)")
		return time.Time{}, "", false
	}
	if !sourceDB.PITREnabled {
		writeError(w, http.StatusConflict, "point-in-time restore is not enabled for this database")
		return time.Time{}, "", false
	}
	target, err := time.Parse(time.RFC3339, req.At)
	if err != nil {
		writeError(w, http.StatusBadRequest, "at must be an RFC3339 timestamp")
		return time.Time{}, "", false
	}
	if err := rt.mysqlPITRRestoreRunner.ValidateTarget(r.Context(), sourceDB.Name, databaseContainerName(sourceDB.Name), target); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return time.Time{}, "", false
	}
	bases, err := rt.baseBackupHistory.ListBaseBackupHistory(r.Context(), sourceDB.Name)
	if err != nil {
		rt.internalError(w, "api: branch database: list base backups failed", err, slog.String("name", sourceDB.Name))
		return time.Time{}, "", false
	}
	var best store.BaseBackupHistory
	var bestAt time.Time
	for _, b := range bases {
		finished, perr := time.Parse(time.RFC3339, b.FinishedAt)
		if b.Status != store.BackupStatusSucceeded || perr != nil || finished.After(target) || !sourceDB.OwnsBackup(b.InstanceID, b.StartedAt) {
			continue
		}
		if best.ID == "" || finished.After(bestAt) {
			best, bestAt = b, finished
		}
	}
	if best.ID == "" {
		writeError(w, http.StatusConflict, "no base backup exists at or before that time")
		return time.Time{}, "", false
	}
	return target, best.ID, true
}

// finishBranchSetup keeps the pooler off: a branch starts with no pooler until
// someone enables it. The size preset is copied at create so quota covers it.
func (rt *Router) finishBranchSetup(w http.ResponseWriter, r *http.Request, sourceDB *store.DesiredDatabase, name string) bool {
	desired, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: branch database: reload new database failed", err, slog.String("name", name))
		return false
	}
	if ps, ok := rt.databases.(databasePoolerStore); ok && desired.PoolerEnabled {
		if err := ps.SetDatabasePooler(r.Context(), name, false); err != nil {
			rt.logger.Warn("api: branch database: disable pooler failed", slog.String("name", name), slog.String("error", err.Error()))
		}
	}
	return true
}

// copyResources returns an independent copy so the new database never aliases the source's.
func copyResources(r *store.ServiceResources) *store.ServiceResources {
	if r == nil {
		return nil
	}
	c := *r
	return &c
}
