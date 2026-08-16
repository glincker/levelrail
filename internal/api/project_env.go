package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// handleGetProjectEnv handles GET /api/v1/projects/{id}/env: the shared
// env vars every app filed under this project inherits as its
// resolveEnv base layer (internal/reconcile/application). A plain map,
// the same wire shape appResource.Env already has.
func (rt *Router) handleGetProjectEnv(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if _, err := rt.projects.GetProject(r.Context(), id); errors.Is(err, store.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	} else if err != nil {
		rt.logger.Error("api: get project env: load project failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	vars, err := rt.projects.ListProjectEnvVars(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: get project env failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, vars)
}

// handleSetProjectEnv handles PUT /api/v1/projects/{id}/env: full
// replace, the same semantics PUT /apps/{name}'s own env field has, not
// a partial patch. See store.DB.SetProjectEnvVars's own doc comment.
func (rt *Router) handleSetProjectEnv(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if _, err := rt.projects.GetProject(r.Context(), id); errors.Is(err, store.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set project env: load project failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var vars map[string]string
	if err := json.NewDecoder(r.Body).Decode(&vars); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if vars == nil {
		vars = map[string]string{}
	}

	if err := rt.projects.SetProjectEnvVars(r.Context(), id, vars); err != nil {
		rt.logger.Error("api: set project env failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, vars)
}
