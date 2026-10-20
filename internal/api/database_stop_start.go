package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// handleStopDatabase handles POST /api/v1/databases/{name}/stop: sets
// Suspended, the database counterpart to handleStopApp. No request
// body. The reconciler (not this handler) is what actually removes the
// container, on its next pass; see
// internal/reconcile/database's own Suspended handling in Reconcile.
// Unlike delete, this keeps the desired database row (and its data
// volume) exactly as they are.
func (rt *Router) handleStopDatabase(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := rt.databases.UpdateDatabaseSuspended(r.Context(), name, true); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: stop database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.reloadAndWriteDatabase(w, r, name, "stop database")
}

// handleStartDatabase handles POST /api/v1/databases/{name}/start:
// clears Suspended, letting the reconciler recreate the container on its
// next pass, against the same data volume it always used. No request
// body.
func (rt *Router) handleStartDatabase(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := rt.databases.UpdateDatabaseSuspended(r.Context(), name, false); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: start database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.reloadAndWriteDatabase(w, r, name, "start database")
}
