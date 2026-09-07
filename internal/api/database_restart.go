package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// databaseRestartStopTimeout bounds how long a restart waits for the
// engine to shut down cleanly before Docker forces it, the same value
// internal/reconcile/database's own container-replace path uses for a
// graceful stop.
const databaseRestartStopTimeout = 10 * time.Second

// handleRestartDatabase handles POST /api/v1/databases/{name}/restart:
// stop then start the database's current container in place, no request
// body. Unlike handleRestartApp, this is synchronous and talks to the
// container directly via execRuntime rather than going through the
// reconciler: a database's container name never changes (containerName's
// own doc comment in internal/reconcile/database explains why, the same
// single-canonical-container-per-volume constraint that rules out an
// app-style restart-nonce), so there is no desired-state field a
// reconcile pass could react to. This mirrors applyResourcesLive's
// direct-to-runtime shape, but reports failure to the caller instead of
// silently degrading, since restart is the whole point of the request.
func (rt *Router) handleRestartDatabase(w http.ResponseWriter, r *http.Request) {
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "database restart is not configured on this control plane")
		return
	}

	name := r.PathValue("name")

	desired, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: restart database: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	runtime, err := rt.execRuntime(desired.NodeID)
	if err != nil {
		rt.logger.Error("api: restart database: resolve node runtime failed",
			slog.String("error", err.Error()), slog.String("name", name), slog.String("node_id", desired.NodeID))
		writeError(w, http.StatusBadGateway, "database's node is not currently reachable")
		return
	}

	target := databaseContainerName(name)
	state, err := runtime.InspectByName(r.Context(), target)
	if err != nil {
		rt.logger.Error("api: restart database: inspect container failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusBadGateway, "could not reach the database's container")
		return
	}
	if state == nil {
		writeError(w, http.StatusConflict, "database has no running container yet")
		return
	}

	if state.Running {
		if err := runtime.Stop(r.Context(), state.ID, databaseRestartStopTimeout); err != nil {
			rt.logger.Error("api: restart database: stop failed", slog.String("error", err.Error()), slog.String("name", name))
			writeError(w, http.StatusBadGateway, "failed to stop the database's container")
			return
		}
	}
	if err := runtime.Start(r.Context(), state.ID); err != nil {
		rt.logger.Error("api: restart database: start failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusBadGateway, "failed to start the database's container")
		return
	}

	rt.reloadAndWriteDatabase(w, r, name, "restart database")
}
