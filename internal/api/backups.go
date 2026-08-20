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
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/cronexpr"
	"github.com/GLINCKER/levelrail/internal/store"
)

// databaseContainerName mirrors internal/reconcile/database's own
// containerName ("db-" + dbName), the same duplicate-the-string-format
// tradeoff databaseControllerName (databases.go) already makes for that
// package's controller-name convention: a real Go import of
// internal/reconcile/database from this package would couple the API
// layer to reconciler internals for the sake of one derived string, and
// this package's own established pattern is to duplicate a small,
// stable format instead. If this format ever changes, both copies need
// updating together; the two existing precedents this mirrors
// (databaseControllerName here, applicationControllerName for apps) have
// held stable since this codebase's first release.
func databaseContainerName(dbName string) string {
	return "db-" + dbName
}

// loadDatabaseForRunner looks up name and writes the 404/500 response
// itself on failure, the shared shape handleTriggerBackup and
// handleTriggerRestore both need before starting a runner.
func (rt *Router) loadDatabaseForRunner(w http.ResponseWriter, r *http.Request, name, logContext string) (*store.DesiredDatabase, bool) {
	db, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return nil, false
	}
	if err != nil {
		rt.logger.Error(logContext, slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil, false
	}
	return db, true
}

// loadBackupTarget confirms targetID exists, writing the 404/500 response
// itself on failure, the shared shape handleTriggerBackup and
// handleSetBackupSchedule both need before using a target.
func (rt *Router) loadBackupTarget(w http.ResponseWriter, r *http.Request, targetID, logContext string) bool {
	if _, err := rt.backupTargets.GetBackupTarget(r.Context(), targetID); errors.Is(err, store.ErrBackupTargetNotFound) {
		writeError(w, http.StatusNotFound, "backup target not found")
		return false
	} else if err != nil {
		rt.logger.Error(logContext, slog.String("error", err.Error()), slog.String("target_id", targetID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}
	return true
}

// BackupHistoryStore is the store surface the backup history handler
// needs. GetBackupHistory was added for handleTriggerRestore
// (restore.go): resolving the one backup attempt a restore names, not
// just listing every attempt for a database, the same "look up one row
// by ID" need GetBackupTarget already serves for backup targets.
type BackupHistoryStore interface {
	ListBackupHistory(ctx context.Context, databaseName string, limit int, before *time.Time) ([]store.BackupHistory, error)
	GetBackupHistory(ctx context.Context, id string) (store.BackupHistory, error)
}

// BackupRunner is the surface the backup trigger handler needs from
// internal/backup.Runner: run one backup attempt end to end, from an
// already-minted history ID through to a finished store.BackupHistory
// row. *backup.Runner satisfies this structurally; internal/api never
// imports internal/backup directly (the same reconciler-internals
// boundary databaseContainerName's own doc comment describes), only this
// narrow interface, wired in by cmd/levelrail at startup.
type BackupRunner interface {
	RunBackup(ctx context.Context, historyID, databaseName, engine, containerName, targetID string) error
}

// backupHistoryResource is the wire shape for one backup attempt.
type backupHistoryResource struct {
	ID           string `json:"id"`
	DatabaseName string `json:"database_name"`
	TargetID     string `json:"target_id"`
	ObjectKey    string `json:"object_key"`
	SizeBytes    int64  `json:"size_bytes"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at,omitempty"`
}

func toBackupHistoryResource(h store.BackupHistory) backupHistoryResource {
	return backupHistoryResource{
		ID:           h.ID,
		DatabaseName: h.DatabaseName,
		TargetID:     h.TargetID,
		ObjectKey:    h.ObjectKey,
		SizeBytes:    h.SizeBytes,
		Status:       h.Status,
		Error:        h.Error,
		StartedAt:    h.StartedAt,
		FinishedAt:   h.FinishedAt,
	}
}

type triggerBackupRequest struct {
	TargetID string `json:"target_id"`
}

// handleTriggerBackup handles POST /api/v1/databases/{name}/backups:
// starts a real backup of name to the target named in the request body
// and returns as soon as the attempt is recorded and under way
// (StatusAccepted, not StatusCreated: the work Runner does, a real
// dump plus a real upload, is not done by the time this handler
// returns, only started), the same "this returns once desired state is
// saved, not once it has actually converged" shape handleTriggerDeploy
// establishes for an app deploy. GET .../backups (handleListBackupHistory
// below) is how a caller finds out whether it actually finished.
//
// AbilityWriteSensitive: this triggers real work against a real bucket
// using a real, previously-stored credential, the same sensitivity
// class creating or deleting a backup target itself already carries.
//
// Runner.RunBackup runs in a goroutine against context.Background(), not
// r.Context(): r.Context() is cancelled the moment this handler returns,
// which would abort the dump and upload within microseconds of starting
// them on every single call, defeating the entire point of returning
// before they finish.
func (rt *Router) handleTriggerBackup(w http.ResponseWriter, r *http.Request) {
	if rt.backupRunner == nil {
		writeError(w, http.StatusNotImplemented, "backups are not configured on this control plane (no master key set)")
		return
	}

	name := r.PathValue("name")

	db, ok := rt.loadDatabaseForRunner(w, r, name, "api: trigger backup: load database failed")
	if !ok {
		return
	}

	var req triggerBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.TargetID == "" {
		writeError(w, http.StatusBadRequest, "target_id is required")
		return
	}

	if !rt.loadBackupTarget(w, r, req.TargetID, "api: trigger backup: load backup target failed") {
		return
	}

	historyID, err := randomBackupHistoryID()
	if err != nil {
		rt.logger.Error("api: trigger backup: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	containerName := databaseContainerName(name)
	go func() { //nolint:gosec // deliberately not r.Context(): it is cancelled the moment this handler returns, which would abort the backup within microseconds of starting it; see this handler's own doc comment
		if err := rt.backupRunner.RunBackup(context.Background(), historyID, name, db.Engine, containerName, req.TargetID); err != nil {
			rt.logger.Error("api: backup run failed", slog.String("error", err.Error()), slog.String("id", historyID), slog.String("database", name))
		}
	}()

	writeJSON(w, http.StatusAccepted, backupHistoryResource{
		ID:           historyID,
		DatabaseName: name,
		TargetID:     req.TargetID,
		Status:       store.BackupStatusRunning,
	})
}

const (
	defaultBackupHistoryLimit = 50
	maxBackupHistoryLimit     = 200
)

// handleListBackupHistory handles GET /api/v1/databases/{name}/backups.
// Cursor-paginated by ?before/?limit, mirroring handleListAuditLog's
// contract since backup history on a long-lived schedule is unbounded.
func (rt *Router) handleListBackupHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.databases.GetDesiredDatabase(r.Context(), name); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: list backup history: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
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

	history, err := rt.backupHistory.ListBackupHistory(r.Context(), name, limit, before)
	if err != nil {
		rt.logger.Error("api: list backup history failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]backupHistoryResource, 0, len(history))
	for _, h := range history {
		out = append(out, toBackupHistoryResource(h))
	}
	writeJSON(w, http.StatusOK, out)
}

// backupScheduleResource is the wire shape for a database's scheduled
// backup config: which target, on what cron schedule, and how many
// successful backups to retain. Deliberately its own small type, not a
// reuse of databaseResource: PUT/DELETE here only ever touch these three
// fields, so echoing back the full database resource (name, engine,
// version, node, project) would suggest a caller could change those
// through this endpoint too, which it cannot.
type backupScheduleResource struct {
	DatabaseName string `json:"database_name"`
	TargetID     string `json:"target_id,omitempty"`
	Schedule     string `json:"schedule,omitempty"`
	Retain       int    `json:"retain,omitempty"`
}

// setBackupScheduleRequest is handleSetBackupSchedule's request body.
type setBackupScheduleRequest struct {
	TargetID string `json:"target_id"`
	Schedule string `json:"schedule"`
	Retain   int    `json:"retain,omitempty"`
}

// handleSetBackupSchedule handles
// PUT /api/v1/databases/{name}/backup-schedule: the missing link wave-2
// roadmap item 6 closes. internal/spec.Backup has carried Schedule and
// Retain since app.yaml's first draft, but nothing ever persisted them;
// this is where an operator (or, eventually, an app.yaml apply path)
// actually wires a database to a backup_targets row, a cron expression,
// and a retention count, so internal/backup.Scheduler has something to
// evaluate on its own tick.
//
// The cron expression is validated synchronously here via
// cronexpr.Parse, the same "validate everything this handler can,
// before ever writing state" discipline handleTriggerRestore's own doc
// comment describes: an operator who fat-fingers a schedule string
// finds out immediately as a 400, not silently on the next scheduler
// tick, internal/backup.Scheduler.Tick's own "log and skip" handling of
// an invalid schedule (see that method's doc comment) exists as a
// defense-in-depth backstop for this check, not as the primary way a
// mistake gets caught.
func (rt *Router) handleSetBackupSchedule(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.databases.GetDesiredDatabase(r.Context(), name); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set backup schedule: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req setBackupScheduleRequest
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

	if !rt.loadBackupTarget(w, r, req.TargetID, "api: set backup schedule: load backup target failed") {
		return
	}

	if err := rt.databases.SetDatabaseBackupSchedule(r.Context(), name, req.TargetID, req.Schedule, req.Retain); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set backup schedule failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, backupScheduleResource{
		DatabaseName: name,
		TargetID:     req.TargetID,
		Schedule:     req.Schedule,
		Retain:       req.Retain,
	})
}

// handleClearBackupSchedule handles
// DELETE /api/v1/databases/{name}/backup-schedule: the reverse of
// handleSetBackupSchedule, returning name to its default "no scheduled
// backup configured" state (store.SetDatabaseBackupSchedule's own "" /
// "" / 0 sentinel values). This never touches store.BackupHistory rows
// already written by past scheduled runs, only the going-forward
// configuration: a database that stops being scheduled keeps its
// backup history exactly like one that was only ever backed up
// manually.
func (rt *Router) handleClearBackupSchedule(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.databases.GetDesiredDatabase(r.Context(), name); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: clear backup schedule: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.databases.SetDatabaseBackupSchedule(r.Context(), name, "", "", 0); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: clear backup schedule failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// randomBackupHistoryID mirrors randomBackupTargetID's exact shape
// (backup_targets.go), which itself mirrors randomNodeJoinTokenID
// (internal/api/nodes.go): 9 random bytes, URL-safe base64, a short type
// prefix. Duplicated rather than shared, the same "different resource,
// different ID space" reasoning both of those give.
func randomBackupHistoryID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate backup history id: %w", err)
	}
	return "bkh_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
