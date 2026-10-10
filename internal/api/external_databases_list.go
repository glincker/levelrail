package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/extdb"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

func ptrTo[T any](v T) *T { return &v }

// externalStatusSummary maps a probe outcome to the same label and variant
// pair the managed databases' status dot uses.
func externalStatusSummary(status string) appStatusSummary {
	switch status {
	case store.ExternalHealthReachable:
		return appStatusSummary{Label: "Reachable", Variant: "success"}
	case store.ExternalHealthSlow:
		return appStatusSummary{Label: "Slow", Variant: "muted"}
	case store.ExternalHealthAuthFailed:
		return appStatusSummary{Label: "Sign-in failed", Variant: "destructive"}
	case store.ExternalHealthTLSError:
		return appStatusSummary{Label: "TLS error", Variant: "destructive"}
	case store.ExternalHealthUnreachable:
		return appStatusSummary{Label: "Unreachable", Variant: "destructive"}
	}
	return appStatusSummary{Label: "Not checked yet", Variant: "muted"}
}

// appendExternalDatabases adds the caller's visible external databases to a
// databases list. They are skipped when the list is filtered by environment,
// since external records do not belong to one.
func (rt *Router) appendExternalDatabases(r *http.Request, canSee func(string) bool, out []databaseListResource) []databaseListResource {
	if r.URL.Query().Get("environment") != "" {
		return out
	}
	recs, err := rt.externalDatabases.ListExternalDatabases(r.Context())
	if err != nil {
		rt.logger.Error("api: list databases: list external databases failed", slog.String("error", err.Error()))
		return out
	}
	for _, d := range recs {
		if !canSee(d.Name) {
			continue
		}
		res := rt.toExternalDatabaseResource(r.Context(), d)
		out = append(out, databaseListResource{
			databaseResource: databaseResource{Name: d.Name, Engine: d.Engine, NodeID: d.NodeID, ProjectID: d.ProjectID},
			Status:           externalStatusSummary(d.HealthStatus),
			External:         &res,
		})
	}
	return out
}

// isExternalDatabase reports whether name is an external database record.
func (rt *Router) isExternalDatabase(r *http.Request, name string) bool {
	_, err := rt.externalDatabases.GetExternalDatabase(r.Context(), name)
	return err == nil
}

// lookupAppDatabase finds the managed or external database an app env var
// names. An external record is returned as a DesiredDatabase carrying only
// its name, engine and node, which is all callers need.
func (rt *Router) lookupAppDatabase(ctx context.Context, name string) (*store.DesiredDatabase, bool, error) {
	d, err := rt.databases.GetDesiredDatabase(ctx, name)
	if !errors.Is(err, store.ErrDatabaseNotFound) {
		return d, false, err
	}
	ext, eerr := rt.externalDatabases.GetExternalDatabase(ctx, name)
	if eerr != nil {
		if errors.Is(eerr, store.ErrExternalDatabaseNotFound) {
			return nil, false, store.ErrDatabaseNotFound
		}
		return nil, false, eerr
	}
	return &store.DesiredDatabase{Name: ext.Name, Engine: ext.Engine, NodeID: ext.NodeID}, true, nil
}

func databaseFieldSupported(external bool, engine, field string) bool {
	if external {
		return extdb.SupportsField(engine, field)
	}
	return database.SupportsField(engine, field)
}

// externalizeConnection rewrites a connection preview for an external
// database: its host is the stored address, never a mesh name.
func (rt *Router) externalizeConnection(ctx context.Context, res appConnectionResource, external bool) appConnectionResource {
	if !external {
		return res
	}
	res.MeshDNS, res.CrossNode = false, false
	if ext, err := rt.externalDatabases.GetExternalDatabase(ctx, res.DatabaseName); err == nil {
		res.Host = ext.Host
	}
	return res
}

// externalConditions presents the last probe as one reconcile-style
// condition so every status badge works without a special case.
func externalConditions(d store.ExternalDatabase) []reconcile.Condition {
	c := reconcile.Condition{Type: "Reachable", Reason: d.HealthStatus, Message: d.HealthReason, Status: reconcile.ConditionUnknown}
	switch d.HealthStatus {
	case store.ExternalHealthReachable, store.ExternalHealthSlow:
		c.Status = reconcile.ConditionTrue
	case store.ExternalHealthAuthFailed, store.ExternalHealthTLSError, store.ExternalHealthUnreachable:
		c.Status = reconcile.ConditionFalse
	}
	if d.HealthStatus == "" {
		return []reconcile.Condition{}
	}
	return []reconcile.Condition{c}
}
