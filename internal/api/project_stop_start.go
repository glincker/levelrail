package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// projectLifecycleResult is the response for POST /api/v1/projects/{id}/stop
// and .../start: which apps and databases in the project succeeded or
// failed the requested state change. Every resource is attempted
// independently, so one failure never aborts the rest, the same
// per-item-keep-going shape a bulk operation across many resources
// needs to report partial success honestly.
type projectLifecycleResult struct {
	SucceededApps      []string `json:"succeeded_apps"`
	SucceededDatabases []string `json:"succeeded_databases"`
	FailedApps         []string `json:"failed_apps,omitempty"`
	FailedDatabases    []string `json:"failed_databases,omitempty"`
}

// handleStopProject handles POST /api/v1/projects/{id}/stop: suspends
// every app and database filed under the project, the project-scoped
// counterpart to handleStopApp/the future per-database stop route. Like
// those, this only sets Suspended; the application and database
// reconcilers are what actually stop containers, on their next pass.
func (rt *Router) handleStopProject(w http.ResponseWriter, r *http.Request) {
	rt.handleProjectLifecycle(w, r, true)
}

// handleStartProject handles POST /api/v1/projects/{id}/start: clears
// Suspended on every app and database filed under the project.
func (rt *Router) handleStartProject(w http.ResponseWriter, r *http.Request) {
	rt.handleProjectLifecycle(w, r, false)
}

func (rt *Router) handleProjectLifecycle(w http.ResponseWriter, r *http.Request, suspended bool) {
	id := r.PathValue("id")

	if _, err := rt.projects.GetProject(r.Context(), id); errors.Is(err, store.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	} else if err != nil {
		rt.logger.Error("api: project lifecycle: get project failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	apps, err := rt.apps.ListDesiredServicesByProject(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: project lifecycle: list apps failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	databases, err := rt.databases.ListDesiredDatabasesByProject(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: project lifecycle: list databases failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	result := projectLifecycleResult{
		SucceededApps:      []string{},
		SucceededDatabases: []string{},
	}
	for _, app := range apps {
		if err := rt.apps.UpdateServiceSuspended(r.Context(), app.Name, suspended); err != nil {
			rt.logger.Error("api: project lifecycle: update app suspended failed",
				slog.String("error", err.Error()), slog.String("project_id", id), slog.String("app", app.Name))
			result.FailedApps = append(result.FailedApps, app.Name)
			continue
		}
		result.SucceededApps = append(result.SucceededApps, app.Name)
	}
	for _, db := range databases {
		if err := rt.databases.UpdateDatabaseSuspended(r.Context(), db.Name, suspended); err != nil {
			rt.logger.Error("api: project lifecycle: update database suspended failed",
				slog.String("error", err.Error()), slog.String("project_id", id), slog.String("database", db.Name))
			result.FailedDatabases = append(result.FailedDatabases, db.Name)
			continue
		}
		result.SucceededDatabases = append(result.SucceededDatabases, db.Name)
	}

	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, result)
}
