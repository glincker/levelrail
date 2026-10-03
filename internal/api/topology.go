package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// handleGetProjectTopology handles GET /api/v1/projects/{id}/topology: a
// diagram-ready graph of how this project's apps, databases, and shared
// volumes actually relate, derived from already-stored desired state
// (see buildTopologyGraph's own doc comment for exactly which field
// backs each edge kind). AbilityRead, the same ordinary boundary
// GET /api/v1/projects/{id} itself uses, not AbilityRoot: reading the
// topology has no fleet-level consequence.
func (rt *Router) handleGetProjectTopology(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if _, err := rt.projects.GetProject(r.Context(), id); errors.Is(err, store.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: get project topology: load project failed", err, slog.String("id", id))
		return
	}

	services, err := rt.apps.ListDesiredServicesByProject(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: get project topology: list services failed", err, slog.String("id", id))
		return
	}
	databases, err := rt.databases.ListDesiredDatabasesByProject(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: get project topology: list databases failed", err, slog.String("id", id))
		return
	}

	appControllers := make([]string, len(services))
	for i, s := range services {
		appControllers[i] = applicationControllerName(s.Name)
	}
	dbControllers := make([]string, len(databases))
	for i, d := range databases {
		dbControllers[i] = databaseControllerName(d.Name)
	}
	allControllers := append(append([]string{}, appControllers...), dbControllers...)

	conditionsByController, err := rt.deploys.GetConditionsForControllers(r.Context(), allControllers)
	if err != nil {
		rt.internalError(w, "api: get project topology: batch load conditions failed", err, slog.String("id", id))
		return
	}

	appStatus := make(map[string]appStatusSummary, len(services))
	for _, s := range services {
		appStatus[s.Name] = summarizeAppConditions(conditionsByController[applicationControllerName(s.Name)])
	}
	dbStatus := make(map[string]appStatusSummary, len(databases))
	for _, d := range databases {
		dbStatus[d.Name] = summarizeAppConditions(conditionsByController[databaseControllerName(d.Name)])
	}

	writeJSON(w, http.StatusOK, buildTopologyGraph(services, databases, appStatus, dbStatus))
}
