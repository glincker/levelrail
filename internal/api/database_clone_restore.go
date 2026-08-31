package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// CloneRestoreHistoryStore is the store surface the clone-restore history
// handler needs, the "restore as new database" counterpart to
// RestoreHistoryStore (restore.go).
type CloneRestoreHistoryStore interface {
	ListCloneRestores(ctx context.Context, sourceDatabaseName string) ([]store.CloneRestore, error)
}

// CloneRestoreRunner is the surface the clone-restore trigger handler
// needs from internal/backup.CloneRestoreRunner: wait for a newly
// created database to come up and restore a backup into it, from an
// already-minted history ID through to a finished store.CloneRestore
// row. *backup.CloneRestoreRunner satisfies this structurally;
// internal/api never imports internal/backup directly, the same
// boundary RestoreRunner's own doc comment describes.
type CloneRestoreRunner interface {
	RunCloneRestore(ctx context.Context, historyID, sourceDatabaseName, newDatabaseName, backupHistoryID, engine, containerName, controllerName string) error
}

// cloneRestoreResource is the wire shape for one "restore as new
// database" attempt.
type cloneRestoreResource struct {
	ID                 string `json:"id"`
	SourceDatabaseName string `json:"source_database_name"`
	NewDatabaseName    string `json:"new_database_name"`
	BackupHistoryID    string `json:"backup_history_id"`
	Status             string `json:"status"`
	Error              string `json:"error,omitempty"`
	StartedAt          string `json:"started_at"`
	FinishedAt         string `json:"finished_at,omitempty"`
}

func toCloneRestoreResource(h store.CloneRestore) cloneRestoreResource {
	return cloneRestoreResource{
		ID:                 h.ID,
		SourceDatabaseName: h.SourceDatabaseName,
		NewDatabaseName:    h.NewDatabaseName,
		BackupHistoryID:    h.BackupHistoryID,
		Status:             h.Status,
		Error:              h.Error,
		StartedAt:          h.StartedAt,
		FinishedAt:         h.FinishedAt,
	}
}

// cloneRestoreRequest is POST /api/v1/databases/{name}/restore-as-new's
// body. Engine is deliberately not settable here: the new database is
// always created with the source database's own current engine, the
// only value a restore into it could ever actually apply against.
// Version/ProjectID/Resources mirror databaseResource's own create-time
// fields exactly, since the new database is created through the same
// path (createDesiredDatabase, databases.go) any other database is.
type cloneRestoreRequest struct {
	BackupID  string                  `json:"backup_id"`
	NewName   string                  `json:"new_name"`
	Version   string                  `json:"version,omitempty"`
	ProjectID string                  `json:"project_id,omitempty"`
	Resources *store.ServiceResources `json:"resources,omitempty"`
}

// handleCloneRestore handles POST /api/v1/databases/{name}/restore-as-new:
// creates a brand-new database (through createDesiredDatabase, the exact
// path handleCreateDatabase itself uses, so there is only ever one way a
// managed database comes into existence in this codebase) and restores a
// previously succeeded backup of name into it, never touching name's own
// live data. This is the safe alternative to handleTriggerRestore
// (restore.go) for the common "test a migration against real data" or
// "stand up a staging copy" workflows: the source database is read only
// once, to confirm the backup actually came from it and to copy its
// current engine, and is never written to.
//
// AbilityWriteSensitive, not AbilityRoot: unlike handleTriggerRestore this
// endpoint never overwrites anything, it only creates a new resource and
// consumes a previously stored backup-target credential to populate it,
// the same sensitivity class handleTriggerBackup already uses for that
// identical "real work against a real bucket using a real stored
// credential" shape. AbilityRoot stays reserved for the one endpoint that
// can actually destroy live data in place.
//
// Every validation this handler can do synchronously, it does
// synchronously, before ever starting the goroutine that does the real
// work, the same discipline handleTriggerRestore's own doc comment
// describes for its endpoint.
func (rt *Router) handleCloneRestore(w http.ResponseWriter, r *http.Request) {
	if rt.cloneRestoreRunner == nil {
		writeError(w, http.StatusNotImplemented, "restores are not configured on this control plane (no master key set)")
		return
	}

	name := r.PathValue("name")

	sourceDB, ok := rt.loadDatabaseForRunner(w, r, name, "api: clone restore: load source database failed")
	if !ok {
		return
	}

	var req cloneRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.BackupID == "" {
		writeError(w, http.StatusBadRequest, "backup_id is required")
		return
	}
	if req.NewName == "" {
		writeError(w, http.StatusBadRequest, "new_name is required")
		return
	}
	if req.NewName == name {
		writeError(w, http.StatusBadRequest, "new_name must differ from the source database's own name")
		return
	}

	backup, err := rt.backupHistory.GetBackupHistory(r.Context(), req.BackupID)
	if errors.Is(err, store.ErrBackupHistoryNotFound) {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: clone restore: load backup history failed", slog.String("error", err.Error()), slog.String("backup_id", req.BackupID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if backup.DatabaseName != name {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("backup %q was taken from database %q, not %q", req.BackupID, backup.DatabaseName, name))
		return
	}
	if backup.Status != store.BackupStatusSucceeded {
		writeError(w, http.StatusConflict, fmt.Sprintf("backup %q has status %q, only a succeeded backup can be restored from", req.BackupID, backup.Status))
		return
	}

	version := req.Version
	if version == "" {
		// Same version as the source database today, the closest
		// analogue to "restore this backup" a caller who didn't
		// specify one could mean: the backup's own data was produced
		// by that version's engine image.
		version = sourceDB.Version
	}

	newDB := databaseResource{
		Name:      req.NewName,
		Engine:    sourceDB.Engine,
		Version:   version,
		ProjectID: req.ProjectID,
	}
	if !rt.createDesiredDatabase(w, r, newDB) {
		return
	}

	if req.Resources != nil {
		// Mirrors handleSetDatabaseResources' own body (databases.go):
		// resources are ordinary desired state but, like every other
		// new database, aren't accepted directly by createDesiredDatabase
		// (see databaseResource.Resources' own field doc comment), so
		// this is the identical trailing step that endpoint would take,
		// applied once up front instead of via a second operator call.
		desired, err := rt.databases.GetDesiredDatabase(r.Context(), req.NewName)
		if err != nil {
			rt.logger.Error("api: clone restore: reload new database failed", slog.String("error", err.Error()), slog.String("name", req.NewName))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		desired.Resources = req.Resources
		if err := rt.databases.SaveDesiredDatabase(r.Context(), *desired); err != nil {
			rt.logger.Error("api: clone restore: set new database resources failed", slog.String("error", err.Error()), slog.String("name", req.NewName))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	historyID, err := randomCloneRestoreID()
	if err != nil {
		rt.logger.Error("api: clone restore: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	containerName := databaseContainerName(req.NewName)
	controllerName := databaseControllerName(req.NewName)
	go func() { //nolint:gosec // deliberately not r.Context(): it is cancelled the moment this handler returns, which would abort the wait-and-restore within microseconds of starting it, the same reasoning handleTriggerRestore's own goroutine gives
		if err := rt.cloneRestoreRunner.RunCloneRestore(context.Background(), historyID, name, req.NewName, req.BackupID, sourceDB.Engine, containerName, controllerName); err != nil {
			rt.logger.Error("api: clone restore run failed", slog.String("error", err.Error()), slog.String("id", historyID), slog.String("source_database", name), slog.String("new_database", req.NewName))
		}
	}()

	writeJSON(w, http.StatusAccepted, cloneRestoreResource{
		ID:                 historyID,
		SourceDatabaseName: name,
		NewDatabaseName:    req.NewName,
		BackupHistoryID:    req.BackupID,
		Status:             store.BackupStatusRunning,
	})
}

// handleListCloneRestores handles
// GET /api/v1/databases/{name}/clone-restores: past "restore as new
// database" attempts sourced from name, newest first, the same
// AbilityRead visibility-only boundary handleListRestoreHistory already
// draws for its own history route.
func (rt *Router) handleListCloneRestores(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.databases.GetDesiredDatabase(r.Context(), name); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: list clone restores: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	history, err := rt.cloneRestoreHistory.ListCloneRestores(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: list clone restores failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]cloneRestoreResource, 0, len(history))
	for _, h := range history {
		out = append(out, toCloneRestoreResource(h))
	}
	writeJSON(w, http.StatusOK, out)
}

// randomCloneRestoreID mirrors randomRestoreHistoryID's exact shape, with
// its own "clr_" prefix so a clone-restore ID is never visually
// confusable with a restore-history ID ("rsh_") or a backup-history ID
// ("bkh_") in a log line or error message.
func randomCloneRestoreID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate clone restore id: %w", err)
	}
	return "clr_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
