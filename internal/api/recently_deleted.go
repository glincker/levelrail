package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"log/slog"
	"net/http"
	"sort"
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
	e, ok := rt.deletedEntry(r, kind, name, snapshot, lastBackup)
	if !ok {
		return
	}
	if err := st.AddRecentlyDeleted(ctx, e); err != nil {
		rt.logger.Warn("api: undo window save failed", slog.String("name", name), slog.String("error", err.Error()))
	}
}

// deletedEntry builds the undo row for a snapshot; false when it cannot be encoded.
func (rt *Router) deletedEntry(r *http.Request, kind, name string, snapshot any, lastBackup string) (store.RecentlyDeleted, bool) {
	b, err := json.Marshal(snapshot)
	if err != nil {
		rt.logger.Warn("api: undo window snapshot failed", slog.String("name", name), slog.String("error", err.Error()))
		return store.RecentlyDeleted{}, false
	}
	_, _, by, _ := rt.currentActor(r)
	return store.RecentlyDeleted{Kind: kind, Name: name, Snapshot: string(b), LastBackup: lastBackup, DeletedBy: by}, true
}

// liveDeleted is GetRecentlyDeleted that treats a row past the window as gone, so a name is not
// held past it. An expired row is purged on the spot, with its kept history.
func (rt *Router) liveDeleted(ctx context.Context, kind, name string) (store.RecentlyDeleted, error) {
	st := rt.recentlyDeleted()
	if st == nil {
		return store.RecentlyDeleted{}, store.ErrRecentlyDeletedNotFound
	}
	window := rt.recentlyDeletedWindow(ctx)
	e, err := st.GetRecentlyDeleted(ctx, kind, name)
	if err != nil {
		return store.RecentlyDeleted{}, err
	}
	if window <= 0 || !e.DeletedAt.Add(window).After(time.Now()) {
		// A failed purge keeps the name reserved: its webhook and data are still there.
		if _, perr := st.PurgeRecentlyDeleted(ctx, time.Now().Add(-window)); perr != nil {
			rt.logger.Warn("api: purge expired undo rows failed", slog.String("error", perr.Error()))
			return store.RecentlyDeleted{}, perr
		}
		return store.RecentlyDeleted{}, store.ErrRecentlyDeletedNotFound
	}
	return e, nil
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
		if !rt.deletedVisibleToCaller(r, e) {
			continue
		}
		res := recentlyDeletedResource{Kind: e.Kind, Name: e.Name, DeletedAt: e.DeletedAt, ExpiresAt: e.DeletedAt.Add(window), DeletedBy: e.DeletedBy}
		if e.LastBackup != "" {
			res.LastBackup = json.RawMessage(e.LastBackup)
		}
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRestoreRecentlyDeleted handles POST /api/v1/recently-deleted/{kind}/{name}/restore.
// It recreates the config and the app's secret values; the function KV token is not restored (mint a new one; its data is kept).
func (rt *Router) handleRestoreRecentlyDeleted(w http.ResponseWriter, r *http.Request) {
	kind, name := r.PathValue("kind"), r.PathValue("name")
	var restoredNeeds []string
	st := rt.recentlyDeleted()
	if st == nil || (kind != store.RecentlyDeletedApp && kind != store.RecentlyDeletedDatabase) {
		writeError(w, http.StatusNotFound, "nothing to restore")
		return
	}
	e, err := rt.liveDeleted(r.Context(), kind, name)
	if errors.Is(err, store.ErrRecentlyDeletedNotFound) {
		writeError(w, http.StatusNotFound, "nothing to restore: the undo window has passed or it was never deleted")
		return
	}
	if err != nil {
		rt.internalError(w, "api: restore recently deleted", err, slog.String("name", name))
		return
	}
	if kind == store.RecentlyDeletedApp {
		if !rt.authorizeAppRestore(w, r, e) {
			return
		}
		unlock := lockAppName(name)
		// Re-read under the lock so a restore never applies an older snapshot beside newer parked secrets.
		cur, lerr := rt.liveDeleted(r.Context(), kind, name)
		switch {
		case errors.Is(lerr, store.ErrRecentlyDeletedNotFound):
			unlock()
			writeError(w, http.StatusNotFound, "nothing to restore: the undo window has passed or it was never deleted")
			return
		case lerr != nil:
			unlock()
			rt.internalError(w, "api: restore recently deleted", lerr, slog.String("name", name))
			return
		}
		err = rt.restoreDeletedApp(r.Context(), cur)
		if err == nil {
			restoredNeeds = undoCategoriesFromSnapshot(cur.Snapshot)
			// The entry stays if the parked copy cannot be erased, so the purge erases it later.
			if serr := rt.dropParkedSecrets(r.Context(), name); serr != nil {
				rt.logger.Warn("api: erase parked secrets failed", slog.String("name", name), slog.String("error", serr.Error()))
			} else if derr := st.DeleteRecentlyDeleted(r.Context(), kind, name); derr != nil {
				rt.logger.Warn("api: clear restored undo entry failed", slog.String("name", name), slog.String("error", derr.Error()))
			}
		}
		unlock()
	} else {
		if !rt.authorizeDatabaseRestore(w, r, e.Name) {
			return
		}
		unlock := lockAppName("database:" + name)
		// Re-read under the lock so a concurrent restore or purge cannot be restored twice.
		cur, lerr := rt.liveDeleted(r.Context(), kind, name)
		switch {
		case errors.Is(lerr, store.ErrRecentlyDeletedNotFound):
			unlock()
			writeError(w, http.StatusNotFound, "nothing to restore: the undo window has passed or it was never deleted")
			return
		case lerr != nil:
			unlock()
			rt.internalError(w, "api: restore recently deleted", lerr, slog.String("name", name))
			return
		}
		err = rt.restoreDeletedDatabase(r.Context(), cur)
		if err == nil {
			// Only the row restored from is cleared; a newer delete of the same name stays.
			if derr := rt.clearRestoredDatabaseEntry(r.Context(), st, cur); derr != nil {
				rt.logger.Warn("api: clear restored undo entry failed", slog.String("name", name), slog.String("error", derr.Error()))
			}
		}
		unlock()
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
	if kind == store.RecentlyDeletedDatabase {
		rt.changes.Publish(changeDatabases, name)
	} else {
		rt.changes.Publish(changeApps, name)
	}
	rt.nudgeReconciler()
	if kind == store.RecentlyDeletedApp {
		writeJSON(w, http.StatusOK, map[string]any{"status": "restored", "kind": kind, "name": name, "needs_attention": rt.restoreAttentionItems(name, restoredNeeds)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored", "kind": kind, "name": name})
}

type errRestoreConflict string

func (e errRestoreConflict) Error() string { return string(e) }

func (rt *Router) restoreDeletedApp(ctx context.Context, e store.RecentlyDeleted) (err error) {
	if _, err := rt.apps.GetDesiredService(ctx, e.Name); err == nil {
		return errRestoreConflict("an app named " + e.Name + " exists again; rename or delete it first")
	} else if !errors.Is(err, store.ErrServiceNotFound) {
		return err
	}
	if other, err := rt.nameTakenByOtherKind(ctx, e.Name, store.NameKindApp, false); err != nil {
		return err
	} else if other {
		return errRestoreConflict(msgNameUnavailable)
	}
	var svc store.DesiredService
	if err := json.Unmarshal([]byte(e.Snapshot), &svc); err != nil {
		return err
	}
	node, project := svc.NodeID, svc.ProjectID
	svc.AppID = ""
	if err := rt.restoreParkedSecrets(ctx, e.Name); err != nil {
		return err
	}
	saved := false
	// A half-restored app would block the retry, so a failure puts it back to deleted; the parked secrets stay.
	defer func() {
		if err == nil {
			return
		}
		bg := context.WithoutCancel(ctx)
		if saved {
			if derr := rt.apps.DeleteDesiredService(store.WithKeptRevisions(bg), e.Name); derr != nil {
				// The app stays live, so its secrets must stay too.
				rt.logger.Warn("api: roll back failed app restore", slog.String("name", e.Name), slog.String("error", derr.Error()))
				return
			}
		}
		if rt.secrets != nil && secrets.AppNamespace(e.Name) == nil {
			_ = rt.secrets.DeleteAll(bg, e.Name)
		}
	}()
	if err := rt.apps.SaveDesiredService(ctx, svc); err != nil {
		var domainTaken *store.ErrDomainTaken
		if errors.As(err, &domainTaken) {
			return errRestoreConflict(domainTaken.Error())
		}
		return err
	}
	saved = true
	if project != "" {
		if err := rt.apps.UpdateServiceProject(ctx, e.Name, project); err != nil {
			return err
		}
	}
	if node != "" {
		if err := rt.apps.UpdateServiceNode(ctx, e.Name, node); err != nil {
			return err
		}
	}
	// The restored app is a new life of the name; the kept webhook follows it.
	if rt.functionTriggers != nil {
		if _, err := rt.functionTriggers.GetFunctionWebhook(ctx, e.Name); err == nil {
			if err := rt.bindFunctionWebhook(ctx, e.Name); err != nil {
				return err
			}
		}
	}
	if svc.EnvironmentID != "" {
		if es, ok := rt.apps.(interface {
			SetServiceEnvironment(ctx context.Context, serviceName, envID string) error
		}); ok {
			if err := es.SetServiceEnvironment(ctx, e.Name, svc.EnvironmentID); err != nil {
				return err
			}
		}
	}
	// Comes back stopped so nothing runs until the owner starts it.
	return rt.apps.UpdateServiceSuspended(ctx, e.Name, true)
}

// clearRestoredDatabaseEntry drops the undo row only while it is still the one restored from.
func (rt *Router) clearRestoredDatabaseEntry(ctx context.Context, st recentlyDeletedStore, e store.RecentlyDeleted) error {
	cur, err := st.GetRecentlyDeleted(ctx, e.Kind, e.Name)
	if errors.Is(err, store.ErrRecentlyDeletedNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !cur.DeletedAt.Equal(e.DeletedAt) {
		return nil
	}
	return st.DeleteRecentlyDeleted(ctx, e.Kind, e.Name)
}

func (rt *Router) restoreDeletedDatabase(ctx context.Context, e store.RecentlyDeleted) (err error) {
	var d store.DesiredDatabase
	if err := json.Unmarshal([]byte(e.Snapshot), &d); err != nil {
		return err
	}
	d.ExistingVolume = "reuse"
	if other, err := rt.nameTakenByOtherKind(ctx, e.Name, store.NameKindDatabase, false); err != nil {
		return err
	} else if other {
		return errRestoreConflict(msgNameUnavailable)
	}
	if err := rt.insertDesiredDatabase(ctx, d); err != nil {
		if errors.Is(err, store.ErrDatabaseExists) {
			return errRestoreConflict("a database named " + e.Name + " exists again; rename or delete it first")
		}
		return err
	}
	// A half-restored row would block the retry with a conflict, so undo the insert on failure.
	defer func() {
		if err != nil {
			if derr := rt.databases.DeleteDesiredDatabase(context.WithoutCancel(ctx), e.Name); derr != nil {
				rt.logger.Warn("api: roll back failed database restore", slog.String("name", e.Name), slog.String("error", derr.Error()))
			}
		}
	}()
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
	cutoff := now.Add(-rt.recentlyDeletedWindow(ctx))
	// Hold each expiring app's name lock so a restore that already re-read its entry finishes first.
	if all, err := st.ListRecentlyDeleted(ctx); err == nil {
		var names []string
		for _, e := range all {
			if e.Kind == store.RecentlyDeletedApp && e.DeletedAt.Before(cutoff) {
				names = append(names, e.Name)
			}
		}
		sort.Strings(names)
		for _, n := range names {
			defer lockAppName(n)()
		}
	}
	return st.PurgeRecentlyDeleted(ctx, cutoff)
}

// RunRecentlyDeletedSweeper runs SweepRecentlyDeleted hourly until ctx ends.
func (rt *Router) RunRecentlyDeletedSweeper(ctx context.Context) error {
	return footprint.Every(ctx, "recently deleted purge", recentlyDeletedSweepEvery, rt.logger, func(ctx context.Context) error {
		_, err := rt.SweepRecentlyDeleted(ctx, time.Now())
		return err
	})
}
