package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ComposeDBBackupStore is the store surface for compose database service schedules and run history.
type ComposeDBBackupStore interface {
	UpsertComposeServiceBackup(ctx context.Context, c store.ComposeServiceBackup) error
	GetComposeServiceBackup(ctx context.Context, app, service string) (store.ComposeServiceBackup, error)
	ListComposeServiceBackups(ctx context.Context, app string) ([]store.ComposeServiceBackup, error)
	DeleteComposeServiceBackup(ctx context.Context, app, service string) error
	ListComposeServiceBackupRuns(ctx context.Context, app, service string, limit int) ([]store.ComposeServiceBackupRun, error)
}

// ComposeDBBackupRunner is the surface the manual trigger needs from backup.ComposeServiceRunner.
type ComposeDBBackupRunner interface {
	RunComposeServiceBackup(ctx context.Context, runID, app, service, engine, targetID string) error
}

// WithComposeDBBackupRunner enables POST .../compose-db-services/{service}/backups.
func WithComposeDBBackupRunner(r ComposeDBBackupRunner) Option {
	return func(rt *Router) { rt.composeDBBackupRunner = r }
}

type composeDBServiceResource struct {
	Service    string `json:"service"`
	Engine     string `json:"engine"`
	Image      string `json:"image"`
	Scheduled  bool   `json:"scheduled"`
	Schedule   string `json:"schedule,omitempty"`
	TargetID   string `json:"target_id,omitempty"`
	Retain     int    `json:"retain,omitempty"`
	RetainDays int    `json:"retain_days,omitempty"`
	Enabled    bool   `json:"enabled"`
}

type composeDBBackupScheduleRequest struct {
	TargetID   string `json:"target_id"`
	Schedule   string `json:"schedule"`
	Retain     int    `json:"retain,omitempty"`
	RetainDays int    `json:"retain_days,omitempty"`
	Enabled    *bool  `json:"enabled,omitempty"`
}

type composeDBBackupRunResource struct {
	ID         string `json:"id"`
	Service    string `json:"service"`
	TargetID   string `json:"target_id"`
	Status     string `json:"status"`
	ObjectKey  string `json:"object_key,omitempty"`
	SizeBytes  int64  `json:"size_bytes"`
	Checksum   string `json:"checksum,omitempty"`
	Error      string `json:"error,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
}

func composeDBScheduleResource(svc composeDBDetected, c *store.ComposeServiceBackup) composeDBServiceResource {
	out := composeDBServiceResource{Service: svc.service, Engine: svc.engine, Image: svc.image}
	if c != nil {
		out.Scheduled = c.BackupSchedule != "" && c.BackupTargetID != ""
		out.Schedule, out.TargetID = c.BackupSchedule, c.BackupTargetID
		out.Retain, out.RetainDays, out.Enabled = c.BackupRetain, c.BackupRetainDays, c.Enabled
	}
	return out
}

type composeDBDetected struct{ service, engine, image string }

// detectComposeDBServices lists the app's services whose image is a supported database engine.
func (rt *Router) detectComposeDBServices(w http.ResponseWriter, r *http.Request, app string) ([]composeDBDetected, bool) {
	svcs, err := rt.appGroups.ListServicesByApp(r.Context(), app)
	if err != nil {
		rt.internalError(w, "api: compose db services: list services failed", err, slog.String("app", app))
		return nil, false
	}
	if len(svcs) == 0 {
		writeError(w, http.StatusNotFound, "app not found")
		return nil, false
	}
	var out []composeDBDetected
	for _, s := range svcs {
		engine, ok := backup.ComposeServiceEngine(s.Image)
		if !ok {
			continue
		}
		out = append(out, composeDBDetected{service: strings.TrimPrefix(s.Name, app+"-"), engine: engine, image: s.Image})
	}
	return out, true
}

// findComposeDBService writes 404 when the service is missing or is not a supported database.
func (rt *Router) findComposeDBService(w http.ResponseWriter, r *http.Request, app, service string) (composeDBDetected, bool) {
	found, ok := rt.detectComposeDBServices(w, r, app)
	if !ok {
		return composeDBDetected{}, false
	}
	for _, d := range found {
		if d.service == service {
			return d, true
		}
	}
	writeError(w, http.StatusNotFound, "service is not a supported database service of this app")
	return composeDBDetected{}, false
}

func (rt *Router) composeDBStoreReady(w http.ResponseWriter) bool {
	if rt.composeDBBackups == nil {
		writeError(w, http.StatusNotImplemented, "compose database backups are not available on this store")
		return false
	}
	return true
}

// handleListComposeDBServices handles GET /api/v1/apps/{name}/compose-db-services.
func (rt *Router) handleListComposeDBServices(w http.ResponseWriter, r *http.Request) {
	if !rt.composeDBStoreReady(w) {
		return
	}
	app := r.PathValue("name")
	found, ok := rt.detectComposeDBServices(w, r, app)
	if !ok {
		return
	}
	rows, err := rt.composeDBBackups.ListComposeServiceBackups(r.Context(), app)
	if err != nil {
		rt.internalError(w, "api: compose db services: list schedules failed", err, slog.String("app", app))
		return
	}
	byService := make(map[string]store.ComposeServiceBackup, len(rows))
	for _, c := range rows {
		byService[c.Service] = c
	}
	out := make([]composeDBServiceResource, 0, len(found))
	for _, d := range found {
		var cfg *store.ComposeServiceBackup
		if c, ok := byService[d.service]; ok {
			cfg = &c
		}
		out = append(out, composeDBScheduleResource(d, cfg))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetComposeDBBackupSchedule handles PUT .../compose-db-services/{service}/backup-schedule.
func (rt *Router) handleSetComposeDBBackupSchedule(w http.ResponseWriter, r *http.Request) {
	if !rt.composeDBStoreReady(w) {
		return
	}
	app, service := r.PathValue("name"), r.PathValue("service")
	det, ok := rt.findComposeDBService(w, r, app, service)
	if !ok {
		return
	}
	var req composeDBBackupScheduleRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if !validateBackupScheduleRequest(w, req.TargetID, req.Schedule, req.Retain, req.RetainDays) {
		return
	}
	if !rt.loadBackupTarget(w, r, req.TargetID, "api: set compose db backup schedule: load backup target failed") {
		return
	}
	cfg := store.ComposeServiceBackup{
		App: app, Service: service, Engine: det.engine, BackupTargetID: req.TargetID,
		BackupSchedule: req.Schedule, BackupRetain: req.Retain, BackupRetainDays: req.RetainDays,
		Enabled: req.Enabled == nil || *req.Enabled,
	}
	if err := rt.composeDBBackups.UpsertComposeServiceBackup(r.Context(), cfg); err != nil {
		rt.internalError(w, "api: set compose db backup schedule failed", err, slog.String("app", app), slog.String("service", service))
		return
	}
	writeJSON(w, http.StatusOK, composeDBScheduleResource(det, &cfg))
}

// handleDeleteComposeDBBackupSchedule handles DELETE .../compose-db-services/{service}/backup-schedule.
func (rt *Router) handleDeleteComposeDBBackupSchedule(w http.ResponseWriter, r *http.Request) {
	if !rt.composeDBStoreReady(w) {
		return
	}
	app, service := r.PathValue("name"), r.PathValue("service")
	err := rt.composeDBBackups.DeleteComposeServiceBackup(r.Context(), app, service)
	if errors.Is(err, store.ErrComposeServiceBackupNotFound) {
		writeError(w, http.StatusNotFound, "no backup schedule for this service")
		return
	}
	if err != nil {
		rt.internalError(w, "api: delete compose db backup schedule failed", err, slog.String("app", app), slog.String("service", service))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListComposeDBBackupRuns handles GET .../compose-db-services/{service}/backups.
func (rt *Router) handleListComposeDBBackupRuns(w http.ResponseWriter, r *http.Request) {
	if !rt.composeDBStoreReady(w) {
		return
	}
	app, service := r.PathValue("name"), r.PathValue("service")
	limit, _, ok := parseBackupHistoryListParams(w, r)
	if !ok {
		return
	}
	runs, err := rt.composeDBBackups.ListComposeServiceBackupRuns(r.Context(), app, service, limit)
	if err != nil {
		rt.internalError(w, "api: list compose db backup runs failed", err, slog.String("app", app), slog.String("service", service))
		return
	}
	out := make([]composeDBBackupRunResource, 0, len(runs))
	for _, run := range runs {
		out = append(out, composeDBBackupRunResource{
			ID: run.ID, Service: run.Service, TargetID: run.TargetID, Status: run.Status, ObjectKey: run.ObjectKey,
			SizeBytes: run.SizeBytes, Checksum: run.Checksum, Error: run.Error, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type composeDBBackupTriggerRequest struct {
	TargetID string `json:"target_id"`
}

// handleTriggerComposeDBBackup handles POST .../compose-db-services/{service}/backups.
func (rt *Router) handleTriggerComposeDBBackup(w http.ResponseWriter, r *http.Request) {
	if !rt.composeDBStoreReady(w) {
		return
	}
	if rt.composeDBBackupRunner == nil {
		writeError(w, http.StatusNotImplemented, "backups are not configured on this control plane (no master key set)")
		return
	}
	app, service := r.PathValue("name"), r.PathValue("service")
	det, ok := rt.findComposeDBService(w, r, app, service)
	if !ok {
		return
	}
	var req composeDBBackupTriggerRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if req.TargetID == "" {
		// Falls back to the schedule's own target so "run now" needs no body.
		if cfg, err := rt.composeDBBackups.GetComposeServiceBackup(r.Context(), app, service); err == nil {
			req.TargetID = cfg.BackupTargetID
		}
	}
	runID, ok := rt.prepareComposeDBTrigger(w, r, req.TargetID)
	if !ok {
		return
	}
	go func() { //nolint:gosec // deliberately not r.Context(): it is cancelled when this handler returns, as in handleTriggerVolumeBackup
		if err := rt.composeDBBackupRunner.RunComposeServiceBackup(context.Background(), runID, app, service, det.engine, req.TargetID); err != nil {
			rt.logger.Error("api: compose db backup run failed", slog.String("error", err.Error()), slog.String("id", runID), slog.String("app", app), slog.String("service", service))
		}
	}()
	writeJSON(w, http.StatusAccepted, composeDBBackupRunResource{ID: runID, Service: service, TargetID: req.TargetID, Status: store.BackupStatusRunning})
}

func (rt *Router) prepareComposeDBTrigger(w http.ResponseWriter, r *http.Request, targetID string) (string, bool) {
	if targetID == "" {
		writeError(w, http.StatusBadRequest, "target_id is required")
		return "", false
	}
	if !rt.loadBackupTarget(w, r, targetID, "api: trigger compose db backup: load backup target failed") {
		return "", false
	}
	if err := backup.CheckWorkDirSpace(); err != nil {
		writeError(w, http.StatusInsufficientStorage, err.Error())
		return "", false
	}
	id, err := backup.RandomComposeRunID()
	if err != nil {
		rt.internalError(w, "api: trigger compose db backup: generate id failed", err)
		return "", false
	}
	return id, true
}
