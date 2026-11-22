package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const recentlyDeletedSweepEvery = time.Hour

// recentlyDeletedStore is the undo-window table; the real store implements it.
type recentlyDeletedStore interface {
	AddRecentlyDeleted(ctx context.Context, e store.RecentlyDeleted) error
	ListRecentlyDeleted(ctx context.Context) ([]store.RecentlyDeleted, error)
	GetRecentlyDeleted(ctx context.Context, kind, name string) (store.RecentlyDeleted, error)
	DeleteRecentlyDeleted(ctx context.Context, kind, name string) error
	PurgeRecentlyDeleted(ctx context.Context, cutoff time.Time) (int, error)
}

func (rt *Router) recentlyDeleted() recentlyDeletedStore {
	st, _ := rt.apps.(recentlyDeletedStore)
	return st
}

func (rt *Router) recentlyDeletedWindow(ctx context.Context) time.Duration {
	days := rt.runtimeSettings.Int(ctx, runtimesettings.KeyRetentionRecentlyDeletedDays)
	return time.Duration(days) * 24 * time.Hour
}

// lastBackupRef is the newest successful backup of a database, as JSON for the undo entry; "" when none.
func (rt *Router) lastBackupRef(ctx context.Context, database string) string {
	if rt.backupHistory == nil {
		return ""
	}
	rows, err := rt.backupHistory.ListBackupHistory(ctx, database, 20, nil)
	if err != nil {
		return ""
	}
	for _, h := range rows {
		if h.Status != store.BackupStatusSucceeded {
			continue
		}
		b, err := json.Marshal(map[string]string{"id": h.ID, "target_id": h.TargetID, "object_key": h.ObjectKey})
		if err != nil {
			return ""
		}
		return string(b)
	}
	return ""
}

// rememberDeleted keeps a snapshot for the undo window. Failures are logged:
// the delete itself already happened and must not be reported as failed.
func (rt *Router) rememberDeleted(ctx context.Context, r *http.Request, kind, name string, snapshot any, lastBackup string) {
	st := rt.recentlyDeleted()
	if st == nil || rt.recentlyDeletedWindow(ctx) <= 0 {
		return
	}
	b, err := json.Marshal(snapshot)
	if err != nil {
		rt.logger.Warn("api: undo window snapshot failed", slog.String("name", name), slog.String("error", err.Error()))
		return
	}
	_, _, by, _ := rt.currentActor(r)
	if err := st.AddRecentlyDeleted(ctx, store.RecentlyDeleted{Kind: kind, Name: name, Snapshot: string(b), LastBackup: lastBackup, DeletedBy: by}); err != nil {
		rt.logger.Warn("api: undo window save failed", slog.String("name", name), slog.String("error", err.Error()))
	}
}

type recentlyDeletedResource struct {
	Kind       string          `json:"kind"`
	Name       string          `json:"name"`
	DeletedAt  time.Time       `json:"deleted_at"`
	ExpiresAt  time.Time       `json:"expires_at"`
	DeletedBy  string          `json:"deleted_by,omitempty"`
	LastBackup json.RawMessage `json:"last_backup,omitempty"`
}

// handleListRecentlyDeleted handles GET /api/v1/recently-deleted.
func (rt *Router) handleListRecentlyDeleted(w http.ResponseWriter, r *http.Request) {
	st := rt.recentlyDeleted()
	if st == nil {
		writeJSON(w, http.StatusOK, []recentlyDeletedResource{})
		return
	}
	rows, err := st.ListRecentlyDeleted(r.Context())
	if err != nil {
		rt.internalError(w, "api: list recently deleted", err)
		return
	}
	window := rt.recentlyDeletedWindow(r.Context())
	out := make([]recentlyDeletedResource, 0, len(rows))
	for _, e := range rows {
		res := recentlyDeletedResource{Kind: e.Kind, Name: e.Name, DeletedAt: e.DeletedAt, ExpiresAt: e.DeletedAt.Add(window), DeletedBy: e.DeletedBy}
		if e.LastBackup != "" {
			res.LastBackup = json.RawMessage(e.LastBackup)
		}
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRestoreRecentlyDeleted handles POST /api/v1/recently-deleted/{kind}/{name}/restore.
// It recreates the config; secrets were erased with the original and must be set again.
func (rt *Router) handleRestoreRecentlyDeleted(w http.ResponseWriter, r *http.Request) {
	kind, name := r.PathValue("kind"), r.PathValue("name")
	st := rt.recentlyDeleted()
	if st == nil || (kind != store.RecentlyDeletedApp && kind != store.RecentlyDeletedDatabase) {
		writeError(w, http.StatusNotFound, "nothing to restore")
		return
	}
	e, err := st.GetRecentlyDeleted(r.Context(), kind, name)
	if errors.Is(err, store.ErrRecentlyDeletedNotFound) {
		writeError(w, http.StatusNotFound, "nothing to restore: the undo window has passed or it was never deleted")
		return
	}
	if err != nil {
		rt.internalError(w, "api: restore recently deleted", err, slog.String("name", name))
		return
	}
	if kind == store.RecentlyDeletedApp {
		err = rt.restoreDeletedApp(r.Context(), e)
	} else {
		err = rt.restoreDeletedDatabase(r.Context(), e)
	}
	var conflict errRestoreConflict
	if errors.As(err, &conflict) {
		writeError(w, http.StatusConflict, conflict.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: restore recently deleted failed", err, slog.String("name", name))
		return
	}
	if err := st.DeleteRecentlyDeleted(r.Context(), kind, name); err != nil {
		rt.logger.Warn("api: clear restored undo entry failed", slog.String("name", name), slog.String("error", err.Error()))
	}
	rt.changes.Publish(changeApps, name)
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored", "kind": kind, "name": name})
}

type errRestoreConflict string

func (e errRestoreConflict) Error() string { return string(e) }

func (rt *Router) restoreDeletedApp(ctx context.Context, e store.RecentlyDeleted) error {
	if _, err := rt.apps.GetDesiredService(ctx, e.Name); err == nil {
		return errRestoreConflict("an app named " + e.Name + " exists again; rename or delete it first")
	} else if !errors.Is(err, store.ErrServiceNotFound) {
		return err
	}
	var svc store.DesiredService
	if err := json.Unmarshal([]byte(e.Snapshot), &svc); err != nil {
		return err
	}
	node, project := svc.NodeID, svc.ProjectID
	svc.AppID = ""
	if err := rt.apps.SaveDesiredService(ctx, svc); err != nil {
		var domainTaken *store.ErrDomainTaken
		if errors.As(err, &domainTaken) {
			return errRestoreConflict(domainTaken.Error())
		}
		return err
	}
	if project != "" {
		if err := rt.apps.UpdateServiceProject(ctx, e.Name, project); err != nil {
			return err
		}
	}
	if node != "" {
		return rt.apps.UpdateServiceNode(ctx, e.Name, node)
	}
	return nil
}

func (rt *Router) restoreDeletedDatabase(ctx context.Context, e store.RecentlyDeleted) error {
	var d store.DesiredDatabase
	if err := json.Unmarshal([]byte(e.Snapshot), &d); err != nil {
		return err
	}
	d.ExistingVolume = "reuse"
	if err := rt.insertDesiredDatabase(ctx, d); err != nil {
		if errors.Is(err, store.ErrDatabaseExists) {
			return errRestoreConflict("a database named " + e.Name + " exists again; rename or delete it first")
		}
		return err
	}
	if d.NodeID != "" {
		if err := rt.databases.UpdateDatabaseNode(ctx, e.Name, d.NodeID); err != nil {
			return err
		}
	}
	if d.ProjectID != "" {
		if err := rt.databases.UpdateDatabaseProject(ctx, e.Name, d.ProjectID); err != nil {
			return err
		}
	}
	if d.BackupTargetID != "" || d.BackupSchedule != "" {
		return rt.databases.SetDatabaseBackupSchedule(ctx, e.Name, d.BackupTargetID, d.BackupSchedule, d.BackupRetain, d.BackupRetainDays)
	}
	return nil
}

// SweepRecentlyDeleted drops entries older than the undo window. At 0 days the window is off and
// anything left is dropped too.
func (rt *Router) SweepRecentlyDeleted(ctx context.Context, now time.Time) (int, error) {
	st := rt.recentlyDeleted()
	if st == nil {
		return 0, nil
	}
	return st.PurgeRecentlyDeleted(ctx, now.Add(-rt.recentlyDeletedWindow(ctx)))
}

// RunRecentlyDeletedSweeper runs SweepRecentlyDeleted hourly until ctx ends.
func (rt *Router) RunRecentlyDeletedSweeper(ctx context.Context) error {
	return footprint.Every(ctx, "recently deleted purge", recentlyDeletedSweepEvery, rt.logger, func(ctx context.Context) error {
		_, err := rt.SweepRecentlyDeleted(ctx, time.Now())
		return err
	})
}
