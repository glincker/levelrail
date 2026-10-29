package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/rehydrate"
	"github.com/GLINCKER/levelrail/internal/store"
)

const rehydrateBackupPageSize = 500

// WithRehydrateStore persists whole-server recovery reports across restarts.
func WithRehydrateStore(s rehydrate.RunStore) Option {
	return func(rt *Router) { rt.rehydrateStore = s }
}

type rehydrateRequest struct {
	AllowStale bool `json:"allow_stale"`
}

// rehydrateInventory reads databases, volumes and backups through the router's own stores.
type rehydrateInventory struct{ rt *Router }

func (i rehydrateInventory) Databases(ctx context.Context) ([]rehydrate.Database, error) {
	dbs, err := i.rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]rehydrate.Database, 0, len(dbs))
	for _, d := range dbs {
		out = append(out, rehydrate.Database{Name: d.Name, Engine: d.Engine, Stateless: !store.EngineSupportsBackups(d.Engine)})
	}
	return out, nil
}

func (i rehydrateInventory) Volumes(ctx context.Context) ([]rehydrate.Volume, error) {
	svcs, err := i.rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, err
	}
	var out []rehydrate.Volume
	for _, s := range svcs {
		prefix := "app-" + s.Name + "-"
		for _, v := range s.Volumes {
			out = append(out, rehydrate.Volume{App: s.Name, Name: strings.TrimPrefix(v.Name, prefix)})
		}
	}
	return out, nil
}

func (i rehydrateInventory) Backups(ctx context.Context) ([]rehydrate.Backup, error) {
	var (
		out    []rehydrate.Backup
		before *time.Time
	)
	for {
		page, err := i.rt.backupHistory.ListAllBackupHistory(ctx, rehydrateBackupPageSize, before)
		if err != nil {
			return nil, err
		}
		for _, h := range page {
			out = append(out, toRehydrateBackup(h))
		}
		if len(page) < rehydrateBackupPageSize {
			return out, nil
		}
		last, perr := time.Parse(time.RFC3339, page[len(page)-1].StartedAt)
		if perr != nil {
			return out, nil
		}
		before = &last
	}
}

func toRehydrateBackup(h store.BackupHistory) rehydrate.Backup {
	b := rehydrate.Backup{ID: h.ID, Succeeded: h.Status == store.BackupStatusSucceeded, SizeBytes: h.SizeBytes}
	if h.ResourceKind == store.BackupResourceKindVolume {
		b.Kind, b.App, b.Volume = rehydrate.KindVolume, h.ServiceName, h.VolumeName
	} else {
		b.Kind, b.Database = rehydrate.KindDatabase, h.DatabaseName
	}
	ts := h.FinishedAt
	if ts == "" {
		ts = h.StartedAt
	}
	b.FinishedAt, _ = time.Parse(time.RFC3339, ts)
	return b
}

// rehydrateRestorer drives the same runners the per-resource restore routes use.
type rehydrateRestorer struct{ rt *Router }

func (r rehydrateRestorer) RestoreDatabase(ctx context.Context, d rehydrate.Database, backupID string) (string, error) {
	if r.rt.restoreRunner == nil {
		return "", rehydrate.ErrNotConfigured
	}
	id, err := randomRestoreHistoryID()
	if err != nil {
		return "", err
	}
	return id, r.rt.restoreRunner.RunRestore(ctx, id, d.Name, backupID, d.Engine, databaseContainerName(d.Name))
}

func (r rehydrateRestorer) RestoreVolume(ctx context.Context, v rehydrate.Volume, backupID string) (string, error) {
	if r.rt.serviceVolumeRestoreRunner == nil {
		return "", rehydrate.ErrNotConfigured
	}
	svc, err := r.rt.apps.GetDesiredService(ctx, v.App)
	if err != nil {
		return "", fmt.Errorf("load app %q: %w", v.App, err)
	}
	docker, ok := store.ServiceVolumeDockerName(*svc, v.Name)
	if !ok {
		return "", store.ErrServiceVolumeNotFound
	}
	id, err := randomRestoreHistoryID()
	if err != nil {
		return "", err
	}
	return id, r.rt.serviceVolumeRestoreRunner.RunVolumeRestore(ctx, id, v.App, v.Name, docker, backupID)
}

func (rt *Router) rehydrateService(w http.ResponseWriter) (*rehydrate.Service, bool) {
	rt.rehydrateOnce.Do(func() {
		cfg, err := rehydrate.ConfigFromEnv(os.LookupEnv)
		if err != nil {
			rt.rehydrateErr = err
			return
		}
		svc := &rehydrate.Service{Inventory: rehydrateInventory{rt}, Config: cfg, Logger: rt.logger}
		if rt.restoreRunner != nil || rt.serviceVolumeRestoreRunner != nil {
			svc.Restorer = rehydrateRestorer{rt}
		}
		if rt.rehydrateStore != nil {
			svc.Store = rt.rehydrateStore
		}
		rt.rehydrateSvc = svc
	})
	if rt.rehydrateErr != nil {
		rt.internalError(w, "api: rehydrate service unavailable", rt.rehydrateErr)
		return nil, false
	}
	return rt.rehydrateSvc, true
}

func decodeRehydrateRequest(r *http.Request) rehydrateRequest {
	var req rehydrateRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	return req
}

// handleRehydratePlan handles POST /api/v1/control-plane/rehydrate/plan: a dry run that restores nothing.
func (rt *Router) handleRehydratePlan(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.rehydrateService(w)
	if !ok {
		return
	}
	rep, err := svc.Plan(r.Context(), rehydrate.PlanOptions{AllowStale: decodeRehydrateRequest(r).AllowStale})
	if err != nil {
		rt.internalError(w, "api: rehydrate plan failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// handleRehydrateRun handles POST /api/v1/control-plane/rehydrate/run: starts the restores and returns 202.
func (rt *Router) handleRehydrateRun(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.rehydrateService(w)
	if !ok {
		return
	}
	rep, err := svc.Start(r.Context(), rehydrate.PlanOptions{AllowStale: decodeRehydrateRequest(r).AllowStale})
	switch {
	case errors.Is(err, rehydrate.ErrNotConfigured):
		writeError(w, http.StatusNotImplemented, "restores are not configured on this control plane (no master key set)")
	case errors.Is(err, rehydrate.ErrBusy):
		writeError(w, http.StatusConflict, "a rehydrate run is already in progress")
	case err != nil:
		rt.internalError(w, "api: rehydrate run failed", err, slog.String("op", "start"))
	default:
		writeJSON(w, http.StatusAccepted, rep)
	}
}

// handleRehydrateReport handles GET /api/v1/control-plane/rehydrate/report: the live or last DR report.
func (rt *Router) handleRehydrateReport(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.rehydrateService(w)
	if !ok {
		return
	}
	rep, found, err := svc.Report(r.Context())
	if err != nil {
		rt.internalError(w, "api: rehydrate report failed", err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "no rehydrate run has been started yet")
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
