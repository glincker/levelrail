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

// cloneNowResource is POST /api/v1/databases/{name}/clone's response
// body: enough to follow the clone's progress through the existing
// backup and clone-restore history endpoints (GET .../backups,
// GET .../clone-restores) without a third, parallel history table just
// for this action.
type cloneNowResource struct {
	SourceDatabaseName string `json:"source_database_name"`
	NewDatabaseName    string `json:"new_database_name"`
	TargetID           string `json:"target_id"`
	BackupHistoryID    string `json:"backup_history_id"`
	CloneRestoreID     string `json:"clone_restore_id"`
}

type cloneNowRequest struct {
	NewName string `json:"new_name"`
	// TargetID is optional: empty defaults to the source database's own
	// BackupTargetID (its scheduled-backup target, if one is set), the
	// same "operator shouldn't have to already know or repeat a value
	// this platform already has on file" reasoning
	// handleSetBackupSchedule's own doc comment gives for its Retain
	// field's zero-means-default convention.
	TargetID string `json:"target_id,omitempty"`
}

// handleCloneDatabaseNow handles POST /api/v1/databases/{name}/clone:
// unlike POST .../restore-as-new (handleCloneRestore), this does not
// require the operator to have already taken a successful backup to
// restore from. It takes one right now, then restores it into a new
// database, as a single action: the two-step "schedule or trigger a
// backup, wait for it to succeed, then go find that history row" flow
// restore-as-new alone requires is real friction for the common "just
// give me a copy of this database" case, competitors this platform is
// otherwise ahead of (Coolify, Dokploy) don't have. See
// internal/api/preview_environments.go's clonePreviewDatabase for this
// endpoint's own core logic (runCloneNow) reused for a different
// caller: an automatic, unattended clone for a pull request preview.
//
// AbilityWriteSensitive, the same tier restore-as-new and trigger-backup
// already use: this starts real work against a real bucket using a
// real stored credential and only ever creates a new resource, never
// touching the source database's own live data.
func (rt *Router) handleCloneDatabaseNow(w http.ResponseWriter, r *http.Request) {
	if rt.backupRunner == nil || rt.cloneRestoreRunner == nil {
		writeError(w, http.StatusNotImplemented, "backups are not configured on this control plane (no master key set)")
		return
	}

	name := r.PathValue("name")
	source, ok := rt.loadDatabaseForRunner(w, r, name, "api: clone database now: load source database failed")
	if !ok {
		return
	}

	var req cloneNowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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

	targetID := req.TargetID
	if targetID == "" {
		targetID = source.BackupTargetID
	}
	if targetID == "" {
		writeError(w, http.StatusBadRequest, "target_id is required: this database has no backup target configured to clone from")
		return
	}
	if !rt.loadBackupTarget(w, r, targetID, "api: clone database now: load backup target failed") {
		return
	}

	if err := rt.createClonedDatabaseShell(r.Context(), req.NewName, source.Engine, source.Version); err != nil {
		if errors.Is(err, errDatabaseNameTaken) {
			writeError(w, http.StatusConflict, "a database with this name already exists")
			return
		}
		rt.logger.Error("api: clone database now: create new database failed", slog.String("error", err.Error()), slog.String("name", req.NewName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	backupID, err := randomBackupHistoryID()
	if err != nil {
		rt.logger.Error("api: clone database now: generate backup id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	cloneID, err := randomCloneRestoreID()
	if err != nil {
		rt.logger.Error("api: clone database now: generate clone-restore id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	go func() { //nolint:gosec // deliberately not r.Context(): it is cancelled the moment this handler returns, the same reasoning handleTriggerBackup's own goroutine gives
		if err := rt.runCloneNowWithIDs(context.Background(), backupID, cloneID, name, source.Engine, targetID, req.NewName); err != nil {
			rt.logger.Error("api: clone database now failed", slog.String("error", err.Error()), slog.String("source_database", name), slog.String("new_database", req.NewName))
		}
	}()

	writeJSON(w, http.StatusAccepted, cloneNowResource{
		SourceDatabaseName: name,
		NewDatabaseName:    req.NewName,
		TargetID:           targetID,
		BackupHistoryID:    backupID,
		CloneRestoreID:     cloneID,
	})
}

// errDatabaseNameTaken is createClonedDatabaseShell's own conflict
// signal, mirroring createDesiredDatabase's identical (inline, not
// separately named) check for the ordinary create-database path.
var errDatabaseNameTaken = errors.New("api: a database with this name already exists")

// createClonedDatabaseShell saves a bare store.DesiredDatabase row for a
// clone target: no ProjectID, resources, or other operator-editable
// field, just enough (name, engine, version) for the new database's own
// container to come up so runCloneNow's restore step has something
// ready to restore into. An operator can set project/resources on the
// result afterward through the same endpoints any other database uses,
// the same "created minimal, refined after" shape handleCloneRestore's
// own doc comment already accepts for its own new database.
func (rt *Router) createClonedDatabaseShell(ctx context.Context, name, engine, version string) error {
	if _, err := rt.databases.GetDesiredDatabase(ctx, name); err == nil {
		return errDatabaseNameTaken
	} else if !errors.Is(err, store.ErrDatabaseNotFound) {
		return fmt.Errorf("check existing database %q: %w", name, err)
	}
	if err := rt.databases.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: name, Engine: engine, Version: version}); err != nil {
		return fmt.Errorf("save desired database %q: %w", name, err)
	}
	return nil
}

// runCloneNowWithIDs is runCloneNow's own backup-and-restore sequence,
// history IDs minted by the caller (the same "generate the ID before
// the first write" convention every other backup/restore trigger in
// this package already follows) so a caller can report them before the
// work finishes. Exported behavior split from runCloneNow only so
// handleCloneDatabaseNow can hand back backupID/cloneID in its 202
// response immediately; runCloneNow itself is for a caller (preview
// deploy) that has no response to write and no use for the IDs.
func (rt *Router) runCloneNowWithIDs(ctx context.Context, backupID, cloneID, sourceName, engine, targetID, newName string) error {
	if err := rt.backupRunner.RunBackup(ctx, backupID, sourceName, engine, databaseContainerName(sourceName), targetID); err != nil {
		return fmt.Errorf("backup source database %q: %w", sourceName, err)
	}
	containerName := databaseContainerName(newName)
	controllerName := databaseControllerName(newName)
	if err := rt.cloneRestoreRunner.RunCloneRestore(ctx, cloneID, sourceName, newName, backupID, engine, containerName, controllerName); err != nil {
		return fmt.Errorf("restore into %q: %w", newName, err)
	}
	return nil
}

// runCloneNow is clonePreviewDatabase's own entry point
// (preview_environments.go): create newName fresh from source's own
// engine/version, take a real backup of source right now (never an
// existing one: a preview must never reuse the same backup two
// different PRs might race to consume, and there may be no successful
// backup yet at all), and restore it into newName. Blocking: the
// caller (a webhook handler already blocking on a full build+deploy)
// awaits this directly rather than detaching a goroutine the way
// handleCloneDatabaseNow above does for its own HTTP caller.
func (rt *Router) runCloneNow(ctx context.Context, sourceName, targetID, newName string) error {
	source, err := rt.databases.GetDesiredDatabase(ctx, sourceName)
	if err != nil {
		return fmt.Errorf("load source database %q: %w", sourceName, err)
	}
	if err := rt.createClonedDatabaseShell(ctx, newName, source.Engine, source.Version); err != nil {
		return fmt.Errorf("create new database %q: %w", newName, err)
	}

	backupID, err := randomBackupHistoryID()
	if err != nil {
		return fmt.Errorf("generate backup id: %w", err)
	}
	cloneID, err := randomCloneRestoreID()
	if err != nil {
		return fmt.Errorf("generate clone-restore id: %w", err)
	}
	return rt.runCloneNowWithIDs(ctx, backupID, cloneID, sourceName, source.Engine, targetID, newName)
}
