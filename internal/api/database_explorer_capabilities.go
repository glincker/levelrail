package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/dbviewer"
	"github.com/GLINCKER/levelrail/internal/store"
)

type databaseExplorerCapabilitiesResource struct {
	dbviewer.EngineCapabilities
	External bool                `json:"external"`
	Limits   explorerLimitsShape `json:"limits"`
}

type explorerLimitsShape struct {
	MaxRows      int `json:"max_rows"`
	MaxBytes     int `json:"max_bytes"`
	MaxCellBytes int `json:"max_cell_bytes"`
	TimeoutMs    int `json:"timeout_ms"`
}

// handleGetDatabaseExplorerCapabilities tells the UI which explorer features an
// engine has, so it renders by capability instead of engine lists.
func (rt *Router) handleGetDatabaseExplorerCapabilities(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var engine string
	external := false
	desired, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	switch {
	case err == nil:
		engine = desired.Engine
	case errors.Is(err, store.ErrDatabaseNotFound):
		rec, eerr := rt.externalDatabases.GetExternalDatabase(r.Context(), name)
		if errors.Is(eerr, store.ErrExternalDatabaseNotFound) {
			writeError(w, http.StatusNotFound, "database not found")
			return
		}
		if eerr != nil {
			rt.internalError(w, "api: explorer capabilities: load external database failed", eerr, slog.String("name", name))
			return
		}
		engine, external = rec.Engine, true
	default:
		rt.internalError(w, "api: explorer capabilities: load database failed", err, slog.String("name", name))
		return
	}
	row := dbviewer.CapabilitiesFor(engine)
	if external {
		row = dbviewer.ExternalCapabilitiesFor(engine)
	}
	lim := dbviewer.LimitsFromEnv(nil)
	writeJSON(w, http.StatusOK, databaseExplorerCapabilitiesResource{
		EngineCapabilities: row,
		External:           external,
		Limits: explorerLimitsShape{
			MaxRows: lim.MaxRows, MaxBytes: lim.MaxBytes,
			MaxCellBytes: lim.MaxCellBytes, TimeoutMs: int(lim.Timeout.Milliseconds()),
		},
	})
}
