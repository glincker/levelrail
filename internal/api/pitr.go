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
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// databaseDataVolumeName mirrors internal/reconcile/database's own
// dataVolumeName ("db-" + dbName + "-data"), the same duplicate-the-
// string-format tradeoff databaseContainerName above already makes for
// that package's container-naming convention.
func databaseDataVolumeName(dbName string) string {
	return "db-" + dbName + "-data"
}

// BaseBackupHistoryStore is the store surface the base backup handlers
// need.
type BaseBackupHistoryStore interface {
	ListBaseBackupHistory(ctx context.Context, databaseName string) ([]store.BaseBackupHistory, error)
}

// BaseBackupRunner is the surface the base backup trigger handler needs
// from internal/backup.BaseBackupRunner: run one physical base backup
// attempt end to end. *backup.BaseBackupRunner satisfies this
// structurally; internal/api never imports internal/backup directly
// (databaseContainerName's own doc comment describes this boundary),
// only this narrow interface, wired in by cmd/levelrail at startup.
type BaseBackupRunner interface {
	RunBaseBackup(ctx context.Context, historyID, databaseName, containerName, targetID string) error
}

// PITRRestoreHistoryStore is the store surface the PITR restore history
// handler needs.
type PITRRestoreHistoryStore interface {
	ListPITRRestoreHistory(ctx context.Context, databaseName string) ([]store.PITRRestoreHistory, error)
}

// PITRRestoreRunner is the surface the PITR restore endpoints need from
// internal/backup.PITRRunner: compute the currently recoverable window,
// validate a target timestamp against it, and run one restore attempt
// end to end. *backup.PITRRunner satisfies this structurally, the same
// boundary BaseBackupRunner above describes.
type PITRRestoreRunner interface {
	// WindowBounds returns the same information *backup.PITRRunner.Window
	// does, destructured into plain values instead of that method's own
	// PITRWindow struct: an interface method's return type must be
	// identical across a package boundary, and internal/api never
	// imports internal/backup's types directly (databaseContainerName's
	// own doc comment describes this boundary), so a struct return here
	// would need its own separately-declared, ever-so-slightly duplicate
	// type for no real benefit over returning the three values plain.
	WindowBounds(ctx context.Context, databaseName, containerName string) (hasBaseBackup bool, start, end time.Time, err error)
	ValidateTarget(ctx context.Context, databaseName, containerName string, target time.Time) error
	RunPITRRestore(ctx context.Context, historyID, databaseName, containerName, dataVolumeName, baseBackupHistoryID string, targetTime time.Time) error
}

func toBaseBackupHistoryResource(h store.BaseBackupHistory) baseBackupHistoryResource {
	return baseBackupHistoryResource{
		ID:           h.ID,
		DatabaseName: h.DatabaseName,
		TargetID:     h.TargetID,
		LSN:          h.LSN,
		SizeBytes:    h.SizeBytes,
		Status:       h.Status,
		Error:        h.Error,
		StartedAt:    h.StartedAt,
		FinishedAt:   h.FinishedAt,
	}
}

type baseBackupHistoryResource struct {
	ID           string `json:"id"`
	DatabaseName string `json:"database_name"`
	TargetID     string `json:"target_id"`
	LSN          string `json:"lsn,omitempty"`
	SizeBytes    int64  `json:"size_bytes"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at,omitempty"`
}

type pitrStatusResource struct {
	Enabled       bool   `json:"enabled"`
	EnabledAt     string `json:"enabled_at,omitempty"`
	HasBaseBackup bool   `json:"has_base_backup"`
	WindowStart   string `json:"window_start,omitempty"`
	WindowEnd     string `json:"window_end,omitempty"`
	WindowError   string `json:"window_error,omitempty"`
}

type pitrRestoreHistoryResource struct {
	ID                  string `json:"id"`
	DatabaseName        string `json:"database_name"`
	BaseBackupHistoryID string `json:"base_backup_history_id"`
	TargetTimestamp     string `json:"target_timestamp"`
	Status              string `json:"status"`
	Error               string `json:"error,omitempty"`
	StartedAt           string `json:"started_at"`
	FinishedAt          string `json:"finished_at,omitempty"`
}

func toPITRRestoreHistoryResource(h store.PITRRestoreHistory) pitrRestoreHistoryResource {
	return pitrRestoreHistoryResource{
		ID:                  h.ID,
		DatabaseName:        h.DatabaseName,
		BaseBackupHistoryID: h.BaseBackupHistoryID,
		TargetTimestamp:     h.TargetTimestamp,
		Status:              h.Status,
		Error:               h.Error,
		StartedAt:           h.StartedAt,
		FinishedAt:          h.FinishedAt,
	}
}

// handleEnablePITR handles POST /api/v1/databases/{name}/pitr: turns on
// WAL archiving going forward, never retroactive (docs/managing-databases.md).
// Only Postgres supports PITR today; any other engine is rejected with
// 400 rather than silently accepted and never actually archiving.
func (rt *Router) handleEnablePITR(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	db, ok := rt.loadDatabaseForRunner(w, r, name, "api: enable pitr: load database failed")
	if !ok {
		return
	}
	if db.Engine != store.EnginePostgres {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("point-in-time restore is only supported for postgres, not %q", db.Engine))
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := rt.databases.SetDatabasePITR(r.Context(), name, true, now); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: enable pitr failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.reloadAndWriteDatabase(w, r, name, "enable pitr")
}

// handleDisablePITR handles DELETE /api/v1/databases/{name}/pitr: turns
// WAL archiving back off. Existing base backups and already-archived WAL
// stay on disk (this never deletes anything, only stops adding to it),
// so a database can still be restored to any point up through when this
// was called; only points after this call become unreachable.
func (rt *Router) handleDisablePITR(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := rt.databases.SetDatabasePITR(r.Context(), name, false, ""); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: disable pitr failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.reloadAndWriteDatabase(w, r, name, "disable pitr")
}

// handleGetPITRStatus handles GET /api/v1/databases/{name}/pitr: whether
// PITR is enabled and, if so, the currently recoverable window.
// WindowError is populated instead of failing the whole request when the
// window can't be computed right now (most commonly: the container isn't
// running); Enabled/EnabledAt/HasBaseBackup are still useful either way.
func (rt *Router) handleGetPITRStatus(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	db, ok := rt.loadDatabaseForRunner(w, r, name, "api: get pitr status: load database failed")
	if !ok {
		return
	}

	resource := pitrStatusResource{Enabled: db.PITREnabled, EnabledAt: db.PITREnabledAt}
	if db.PITREnabled && rt.pitrRestoreRunner != nil {
		hasBaseBackup, start, end, err := rt.pitrRestoreRunner.WindowBounds(r.Context(), name, databaseContainerName(name))
		if err != nil {
			resource.WindowError = err.Error()
		} else {
			resource.HasBaseBackup = hasBaseBackup
			resource.WindowStart = start.UTC().Format(time.RFC3339)
			resource.WindowEnd = end.UTC().Format(time.RFC3339)
		}
	}

	writeJSON(w, http.StatusOK, resource)
}

type triggerBaseBackupRequest struct {
	TargetID string `json:"target_id"`
}

// handleTriggerBaseBackup handles POST /api/v1/databases/{name}/base-backups:
// starts a real physical base backup of name to the target named in the
// request body, the point-in-time-restore counterpart of
// handleTriggerBackup (backups.go). Same StatusAccepted-before-the-work-
// finishes shape, same AbilityWriteSensitive tier: real work against a
// real bucket using a real, previously-stored credential, no more
// destructive than an ordinary backup.
func (rt *Router) handleTriggerBaseBackup(w http.ResponseWriter, r *http.Request) {
	if rt.baseBackupRunner == nil {
		writeError(w, http.StatusNotImplemented, "backups are not configured on this control plane (no master key set)")
		return
	}

	name := r.PathValue("name")

	db, ok := rt.loadDatabaseForRunner(w, r, name, "api: trigger base backup: load database failed")
	if !ok {
		return
	}
	if !db.PITREnabled {
		writeError(w, http.StatusConflict, "point-in-time restore is not enabled for this database")
		return
	}

	var req triggerBaseBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	historyID, ok := rt.prepareBackupTrigger(w, r, req.TargetID, "api: trigger base backup")
	if !ok {
		return
	}

	containerName := databaseContainerName(name)
	go func() { //nolint:gosec // deliberately not r.Context(): see handleTriggerBackup's own identical reasoning, backups.go
		if err := rt.baseBackupRunner.RunBaseBackup(context.Background(), historyID, name, containerName, req.TargetID); err != nil {
			rt.logger.Error("api: base backup run failed", slog.String("error", err.Error()), slog.String("id", historyID), slog.String("database", name))
		}
	}()

	writeJSON(w, http.StatusAccepted, baseBackupHistoryResource{
		ID:           historyID,
		DatabaseName: name,
		TargetID:     req.TargetID,
		Status:       store.BackupStatusRunning,
	})
}

// handleListBaseBackupHistory handles GET /api/v1/databases/{name}/base-backups.
func (rt *Router) handleListBaseBackupHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, ok := rt.loadDatabaseForRunner(w, r, name, "api: list base backup history: load database failed"); !ok {
		return
	}

	history, err := rt.baseBackupHistory.ListBaseBackupHistory(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: list base backup history failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]baseBackupHistoryResource, 0, len(history))
	for _, h := range history {
		out = append(out, toBaseBackupHistoryResource(h))
	}
	writeJSON(w, http.StatusOK, out)
}

type triggerPITRRestoreRequest struct {
	BaseBackupID string `json:"base_backup_id"`
	TargetTime   string `json:"target_time"`
}

// handleTriggerPITRRestore handles POST /api/v1/databases/{name}/pitr-restore:
// starts a real point-in-time restore to target_time. AbilityRoot, the
// same tier handleTriggerRestore uses and for the same reason. Validated
// synchronously (target_time outside the recoverable window is 409
// immediately) so a bad request never reaches Postgres's own recovery,
// which would hang rather than fail on one (ADR 016).
func (rt *Router) handleTriggerPITRRestore(w http.ResponseWriter, r *http.Request) {
	if rt.pitrRestoreRunner == nil {
		writeError(w, http.StatusNotImplemented, "restores are not configured on this control plane (no master key set)")
		return
	}

	name := r.PathValue("name")

	db, ok := rt.loadDatabaseForRunner(w, r, name, "api: trigger pitr restore: load database failed")
	if !ok {
		return
	}
	if !db.PITREnabled {
		writeError(w, http.StatusConflict, "point-in-time restore is not enabled for this database")
		return
	}

	var req triggerPITRRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.BaseBackupID == "" {
		writeError(w, http.StatusBadRequest, "base_backup_id is required")
		return
	}
	target, err := time.Parse(time.RFC3339, req.TargetTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "target_time must be an RFC3339 timestamp")
		return
	}

	containerName := databaseContainerName(name)
	if err := rt.pitrRestoreRunner.ValidateTarget(r.Context(), name, containerName, target); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	historyID, err := randomPITRRestoreHistoryID()
	if err != nil {
		rt.logger.Error("api: trigger pitr restore: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	dataVolumeName := databaseDataVolumeName(name)
	go func() { //nolint:gosec // deliberately not r.Context(): see handleTriggerRestore's own identical reasoning, restore.go
		if err := rt.pitrRestoreRunner.RunPITRRestore(context.Background(), historyID, name, containerName, dataVolumeName, req.BaseBackupID, target); err != nil {
			rt.logger.Error("api: pitr restore run failed", slog.String("error", err.Error()), slog.String("id", historyID), slog.String("database", name))
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

// handleListPITRRestoreHistory handles GET /api/v1/databases/{name}/pitr-restores.
func (rt *Router) handleListPITRRestoreHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, ok := rt.loadDatabaseForRunner(w, r, name, "api: list pitr restore history: load database failed"); !ok {
		return
	}

	history, err := rt.pitrRestoreHistory.ListPITRRestoreHistory(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: list pitr restore history failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]pitrRestoreHistoryResource, 0, len(history))
	for _, h := range history {
		out = append(out, toPITRRestoreHistoryResource(h))
	}
	writeJSON(w, http.StatusOK, out)
}

// randomPITRRestoreHistoryID mirrors randomRestoreHistoryID's exact
// shape (restore.go), with its own "pitr_" prefix so a PITR restore
// history ID is never visually confusable with an ordinary restore's.
func randomPITRRestoreHistoryID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate pitr restore history id: %w", err)
	}
	return "pitr_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
