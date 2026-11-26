package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/dbintegrations"
)

type databaseIntegrationsResource struct {
	Database     string                      `json:"database"`
	Engine       string                      `json:"engine"`
	Integrations []dbintegrations.CardStatus `json:"integrations"`
}

// handleListDatabaseIntegrations handles GET /api/v1/databases/{name}/integrations:
// the card catalog with each card's status for this database.
func (rt *Router) handleListDatabaseIntegrations(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	db, ok := rt.loadDatabaseForRunner(w, r, name, "api: database integrations: load database failed")
	if !ok {
		return
	}
	var providers []dbintegrations.Provider
	if rt.dataExport != nil {
		providers = append(providers, dbintegrations.NewDatadogProvider(rt.dataExport), dbintegrations.NewOpenTelemetryProvider(rt.dataExport))
	}
	cards, err := dbintegrations.NewService(rt.logger, providers...).List(r.Context(), dbintegrations.Database{Name: name, Engine: db.Engine})
	if err != nil {
		rt.internalError(w, "api: database integrations: list failed", err)
		return
	}
	writeJSON(w, http.StatusOK, databaseIntegrationsResource{Database: name, Engine: db.Engine, Integrations: cards})
}
