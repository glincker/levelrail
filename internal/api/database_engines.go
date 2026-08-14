package api

import (
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// databaseEngineResource is the wire shape for one supported database
// engine: store.DatabaseEngineInfo plus its own JSON tags, the same
// direct-marshal choice appResource/databaseResource already make for
// their own store types.
type databaseEngineResource struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	DefaultVersion string `json:"default_version"`
}

// handleListDatabaseEngines handles GET /api/v1/database-engines: every
// database engine this control plane can actually create
// (store.SupportedDatabaseEngines, backed by the embedded
// internal/store/database_engines.yaml registry, cross-checked against
// internal/reconcile/database.Controller.Reconcile's own switch cases by
// that package's own test). Frontend consumers (the creation wizard's
// step 1 picker) read this instead of hardcoding the engine list as TS
// literals, so a new engine appears in the picker the moment it's added
// to the registry and given a real reconciler case, no frontend code
// change needed for the list itself.
func (rt *Router) handleListDatabaseEngines(w http.ResponseWriter, _ *http.Request) {
	engines, err := store.SupportedDatabaseEngines()
	if err != nil {
		rt.logger.Error("api: list database engines failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]databaseEngineResource, 0, len(engines))
	for _, e := range engines {
		out = append(out, databaseEngineResource{
			ID:             e.ID,
			Label:          e.Label,
			DefaultVersion: e.DefaultVersion,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
