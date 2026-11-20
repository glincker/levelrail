package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// handleListProjectApps handles GET /api/v1/projects/{id}/apps: the app list
// narrowed to one project, with the same visibility and paging rules.
func (rt *Router) handleListProjectApps(w http.ResponseWriter, r *http.Request) {
	if r, ok := rt.projectListRequest(w, r); ok {
		rt.handleListApps(w, r)
	}
}

// handleListProjectDatabases handles GET /api/v1/projects/{id}/databases.
func (rt *Router) handleListProjectDatabases(w http.ResponseWriter, r *http.Request) {
	if r, ok := rt.projectListRequest(w, r); ok {
		rt.handleListDatabases(w, r)
	}
}

// projectListRequest 404s an unknown project and returns the request with the
// project filter set, so both lists reuse the global list handlers.
func (rt *Router) projectListRequest(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	id := r.PathValue("id")
	if _, err := rt.projects.GetProject(r.Context(), id); errors.Is(err, store.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return nil, false
	} else if err != nil {
		rt.internalError(w, "api: list project resources: load project failed", err, slog.String("id", id))
		return nil, false
	}
	r = r.Clone(r.Context())
	q := r.URL.Query()
	q.Set("project", id)
	r.URL.RawQuery = q.Encode()
	return r, true
}
