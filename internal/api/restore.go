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

// RestoreHistoryStore is the store surface the restore history handler
// needs.
type RestoreHistoryStore interface {
	ListRestoreHistory(ctx context.Context, databaseName string) ([]store.RestoreHistory, error)
}

// RestoreRunner is the surface the restore trigger handler needs from
// internal/backup.RestoreRunner: run one restore attempt end to end, from
// an already-minted history ID through to a finished
// store.RestoreHistory row. *backup.RestoreRunner satisfies this
// structurally; internal/api never imports internal/backup directly, the
// same boundary BackupRunner's own doc comment describes.
type RestoreRunner interface {
	RunRestore(ctx context.Context, historyID, databaseName, backupHistoryID, engine, containerName string) error
}

// restoreHistoryResource is the wire shape for one restore attempt.
type restoreHistoryResource struct {
	ID              string `json:"id"`
	DatabaseName    string `json:"database_name"`
	BackupHistoryID string `json:"backup_history_id"`
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at,omitempty"`
}

func toRestoreHistoryResource(h store.RestoreHistory) restoreHistoryResource {
	return restoreHistoryResource{
		ID:              h.ID,
		DatabaseName:    h.DatabaseName,
		BackupHistoryID: h.BackupHistoryID,
		Status:          h.Status,
		Error:           h.Error,
		StartedAt:       h.StartedAt,
		FinishedAt:      h.FinishedAt,
	}
}

type triggerRestoreRequest struct {
	BackupID string `json:"backup_id"`
}

// handleTriggerRestore handles POST /api/v1/databases/{name}/restore:
// starts a real restore of name from a previously succeeded backup
// attempt and returns as soon as the attempt is recorded and under way
// (StatusAccepted, the same "this returns once desired state is saved,
// not once it has actually converged" shape handleTriggerBackup already
// establishes for the forward direction). GET .../restores
// (handleListRestoreHistory below) is how a caller finds out whether it
// actually finished.
//
// AbilityRoot, not AbilityWriteSensitive: this is the single most
// destructive endpoint in this API. It overwrites a live database's
// actual data, in place, with no way to undo it short of restoring
// again from a different backup. AbilityWriteSensitive already covers
// triggering an ordinary backup (real work against a real bucket using a
// real stored credential) and creating/deleting a backup target itself;
// a restore is a strictly higher-stakes operation than either, on the
// same tier as node management and placement (PUT .../node,
// POST /nodes/join-tokens), which is why it gets the same AbilityRoot
// gate those routes already use rather than reusing
// AbilityWriteSensitive just because backups live in the same file.
//
// Every validation this handler can do synchronously, it does
// synchronously, before ever starting the goroutine that does the real
// work: a database or backup ID that doesn't exist, or a backup that
// exists but didn't succeed, is reported immediately as a real HTTP
// status the caller can act on, not silently discovered only once
// GET .../restores is polled later. RestoreRunner.RunRestore repeats the
// backup-succeeded check on its own before doing anything (see its own
// doc comment): this handler's check is what makes the difference
// visible to the caller as a proper 409 instead of a "restore" that
// immediately shows up failed in the history list for a reason the
// caller had no chance to see coming.
func (rt *Router) handleTriggerRestore(w http.ResponseWriter, r *http.Request) {
	if rt.restoreRunner == nil {
		writeError(w, http.StatusNotImplemented, "restores are not configured on this control plane (no master key set)")
		return
	}

	name := r.PathValue("name")

	db, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: trigger restore: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req triggerRestoreRequest
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
		rt.logger.Error("api: trigger restore: load backup history failed", slog.String("error", err.Error()), slog.String("backup_id", req.BackupID))
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

	historyID, err := randomRestoreHistoryID()
	if err != nil {
		rt.logger.Error("api: trigger restore: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	containerName := databaseContainerName(name)
	go func() { //nolint:gosec // deliberately not r.Context(): it is cancelled the moment this handler returns, which would abort the restore within microseconds of starting it, the same reasoning handleTriggerBackup's own goroutine gives
		if err := rt.restoreRunner.RunRestore(context.Background(), historyID, name, req.BackupID, db.Engine, containerName); err != nil {
			rt.logger.Error("api: restore run failed", slog.String("error", err.Error()), slog.String("id", historyID), slog.String("database", name))
		}
	}()

	writeJSON(w, http.StatusAccepted, restoreHistoryResource{
		ID:              historyID,
		DatabaseName:    name,
		BackupHistoryID: req.BackupID,
		Status:          store.BackupStatusRunning,
	})
}

// handleListRestoreHistory handles GET /api/v1/databases/{name}/restores.
// AbilityRead: passive visibility into attempts already made, the same
// boundary handleListBackupHistory already draws between visibility and
// the mutation that starts one.
func (rt *Router) handleListRestoreHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.databases.GetDesiredDatabase(r.Context(), name); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: list restore history: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	history, err := rt.restoreHistory.ListRestoreHistory(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: list restore history failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]restoreHistoryResource, 0, len(history))
	for _, h := range history {
		out = append(out, toRestoreHistoryResource(h))
	}
	writeJSON(w, http.StatusOK, out)
}

// randomRestoreHistoryID mirrors randomBackupHistoryID's exact shape,
// the same "different resource, different ID space" reasoning that
// function's own doc comment gives, with its own "rsh_" prefix so a
// restore history ID and a backup history ID are never visually
// confusable in a log line or an error message even though the two
// often appear together (a restore attempt's own ID and the backup_id it
// names).
func randomRestoreHistoryID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate restore history id: %w", err)
	}
	return "rsh_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
