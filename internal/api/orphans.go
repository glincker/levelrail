package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/orphans"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// orphanSource adapts the router's stores to orphans.Source.
type orphanSource struct{ rt *Router }

func (s orphanSource) ListDesiredServices(ctx context.Context) ([]store.DesiredService, error) {
	return s.rt.apps.ListDesiredServices(ctx)
}

func (s orphanSource) ListDesiredDatabases(ctx context.Context) ([]store.DesiredDatabase, error) {
	return s.rt.databases.ListDesiredDatabases(ctx)
}

func (s orphanSource) ListPendingTeardowns(ctx context.Context) ([]store.PendingTeardown, error) {
	if ts, ok := s.rt.apps.(application.TeardownStore); ok {
		return ts.ListPendingTeardowns(ctx)
	}
	return nil, nil
}

// loadOrphanDesired is the desired-state snapshot every orphan check shares
// with the reaper, with node IDs normalised the way the container list sees them.
func (rt *Router) loadOrphanDesired(ctx context.Context) (orphans.Desired, error) {
	return orphans.LoadDesired(ctx, orphanSource{rt}, func(id string) string {
		if rt.isLocalNode(id) {
			return ""
		}
		return id
	})
}

// handleListOrphans handles GET /api/v1/system/orphans: every leftover
// container and volume across nodes, with where each stands against the
// grace period. Read only.
func (rt *Router) handleListOrphans(w http.ResponseWriter, r *http.Request) {
	if rt.orphanReaper == nil {
		writeError(w, http.StatusNotImplemented, "orphan detection is not configured on this control plane")
		return
	}
	rep, err := rt.orphanReaper.Scan(r.Context())
	if err != nil {
		rt.internalError(w, "api: scan orphans failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// handleReapOrphans handles POST /api/v1/system/orphans/reap: one reaper
// pass on demand. dry_run=true reports what would be removed and removes
// nothing. Resources still inside the grace period are never removed.
func (rt *Router) handleReapOrphans(w http.ResponseWriter, r *http.Request) {
	if rt.orphanReaper == nil {
		writeError(w, http.StatusNotImplemented, "orphan detection is not configured on this control plane")
		return
	}
	dry := r.URL.Query().Get("dry_run") == "true"
	rep, err := rt.orphanReaper.Reap(r.Context(), dry)
	if err != nil {
		rt.internalError(w, "api: reap orphans failed", err, slog.Bool("dry_run", dry))
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
