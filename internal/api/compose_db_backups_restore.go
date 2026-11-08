package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AuditActionComposeDBRestore marks the outcome row of an in-place compose database restore.
const AuditActionComposeDBRestore = "compose_db.restore"

// ComposeDBRestoreRunner is the surface the restore route needs from backup.ComposeServiceRestorer.
type ComposeDBRestoreRunner interface {
	RunComposeServiceRestore(ctx context.Context, safetyRunID, app, service, engine string, run store.ComposeServiceBackupRun) error
}

// WithComposeDBRestoreRunner enables POST .../compose-db-services/{service}/backups/{run}/restore.
func WithComposeDBRestoreRunner(r ComposeDBRestoreRunner) Option {
	return func(rt *Router) { rt.composeDBRestoreRunner = r }
}

// composeRestoreInFlight holds one key per app/service with a restore running, so a retry
// cannot interleave a second replay with the first. Package level to avoid touching the Router.
var composeRestoreInFlight sync.Map

type composeDBRestoreRequest struct {
	Confirm string `json:"confirm"`
	Preview bool   `json:"preview"`
	AsNew   string `json:"as_new,omitempty"`
}

type composeDBRestoreResource struct {
	RunID         string `json:"run_id"`
	ObjectKey     string `json:"object_key"`
	SizeBytes     int64  `json:"size_bytes"`
	Engine        string `json:"engine"`
	App           string `json:"app"`
	Service       string `json:"service"`
	ConfirmPhrase string `json:"confirm_phrase"`
	Preview       bool   `json:"preview"`
	SafetyRunID   string `json:"safety_run_id,omitempty"`
}

// handleRestoreComposeDBBackup handles POST .../compose-db-services/{service}/backups/{run}/restore.
// AbilityRoot: it replaces the live database. Preview mutates nothing.
func (rt *Router) handleRestoreComposeDBBackup(w http.ResponseWriter, r *http.Request) {
	if !rt.composeDBStoreReady(w) {
		return
	}
	if rt.composeDBRestoreRunner == nil {
		writeError(w, http.StatusNotImplemented, "restores are not configured on this control plane (no master key set)")
		return
	}
	app, service, runID := r.PathValue("name"), r.PathValue("service"), r.PathValue("run")
	det, ok := rt.findComposeDBService(w, r, app, service)
	if !ok {
		return
	}
	var req composeDBRestoreRequest
	if !decodeReqLimit(w, r, &req, stepUpBodyLimit) {
		return
	}
	if req.AsNew != "" {
		writeError(w, http.StatusBadRequest, "restoring into a new service is not supported")
		return
	}
	if !backup.ComposeRestoreEngineSupported(det.engine) {
		writeError(w, http.StatusBadRequest, "restore is not supported for this engine")
		return
	}
	run, ok := rt.findComposeDBRun(w, r, app, service, runID)
	if !ok {
		return
	}
	if run.Status != store.BackupStatusSucceeded || run.ObjectKey == "" {
		writeError(w, http.StatusConflict, "only a succeeded backup run can be restored")
		return
	}
	phrase := backup.ComposeRestorePhrase(app, service)
	res := composeDBRestoreResource{
		RunID: run.ID, ObjectKey: run.ObjectKey, SizeBytes: run.SizeBytes, Engine: det.engine,
		App: app, Service: service, ConfirmPhrase: phrase, Preview: req.Preview,
	}
	if req.Preview {
		writeJSON(w, http.StatusOK, res)
		return
	}
	if req.Confirm != phrase {
		writeError(w, http.StatusBadRequest, "confirm must equal the confirm phrase for this service")
		return
	}
	if err := backup.CheckWorkDirSpace(); err != nil {
		writeError(w, http.StatusInsufficientStorage, err.Error())
		return
	}
	safetyID, err := backup.RandomComposeRunID()
	if err != nil {
		rt.internalError(w, "api: restore compose db: generate id failed", err)
		return
	}
	flightKey := app + "/" + service
	if _, busy := composeRestoreInFlight.LoadOrStore(flightKey, safetyID); busy {
		writeError(w, http.StatusConflict, "a restore is already running for this service")
		return
	}
	actorType, actorID, actorName, _ := rt.currentActor(r)
	path := r.URL.Path
	go func() { //nolint:gosec // deliberately not r.Context(): it is cancelled when this handler returns, as in handleTriggerComposeDBBackup
		defer composeRestoreInFlight.Delete(flightKey)
		err := rt.composeDBRestoreRunner.RunComposeServiceRestore(context.Background(), safetyID, app, service, det.engine, run)
		outcome, status := "succeeded", http.StatusOK
		if err != nil {
			outcome, status = "failed", http.StatusInternalServerError
			rt.logger.Error("api: compose db restore failed", slog.String("error", err.Error()), slog.String("run", run.ID), slog.String("app", app), slog.String("service", service))
		}
		rt.auditComposeDBRestore(path, actorType, actorID, actorName, map[string]string{"app": app, "service": service, "run": run.ID, "safety_run": safetyID, "outcome": outcome}, status)
	}()
	res.SafetyRunID = safetyID
	writeJSON(w, http.StatusAccepted, res)
}

// findComposeDBRun looks the run up in the service's recent history; it never trusts the path to name a foreign service's run.
func (rt *Router) findComposeDBRun(w http.ResponseWriter, r *http.Request, app, service, runID string) (store.ComposeServiceBackupRun, bool) {
	runs, err := rt.composeDBBackups.ListComposeServiceBackupRuns(r.Context(), app, service, maxBackupHistoryLimit)
	if err != nil {
		rt.internalError(w, "api: restore compose db: list runs failed", err, slog.String("app", app), slog.String("service", service))
		return store.ComposeServiceBackupRun{}, false
	}
	for _, run := range runs {
		if run.ID == runID {
			return run, true
		}
	}
	writeError(w, http.StatusNotFound, "backup run not found")
	return store.ComposeServiceBackupRun{}, false
}

// auditComposeDBRestore records the async outcome; the route middleware already audited the request itself.
func (rt *Router) auditComposeDBRestore(path, actorType, actorID, actorName string, detail map[string]string, status int) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: compose db restore audit id failed")
		return
	}
	raw, _ := json.Marshal(detail)
	entry := store.AuditEntry{
		ID: id, ActorType: actorType, ActorID: actorID, ActorName: actorName,
		Ability: AbilityRoot, Method: http.MethodPost, Path: path, StatusCode: status,
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: "api",
		Action: AuditActionComposeDBRestore, Detail: string(raw),
	}
	if err := rt.auditLog.SaveAuditEntry(context.Background(), entry); err != nil {
		rt.logger.Warn("api: compose db restore audit save failed", slog.String("error", err.Error()))
		return
	}
	rt.emitAuditEvent(entry)
}
