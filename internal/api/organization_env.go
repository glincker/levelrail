package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// handleGetOrganizationEnv handles GET /api/v1/organizations/{id}/env,
// mirroring handleGetProjectEnv one tier up.
func (rt *Router) handleGetOrganizationEnv(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if _, err := rt.organizations.GetOrganization(r.Context(), id); errors.Is(err, store.ErrOrganizationNotFound) {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	} else if err != nil {
		rt.logger.Error("api: get organization env: load organization failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	vars, err := rt.organizations.ListOrganizationEnvVars(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: get organization env failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, vars)
}

// handleSetOrganizationEnv handles PUT /api/v1/organizations/{id}/env:
// full replace, mirroring handleSetProjectEnv one tier up.
func (rt *Router) handleSetOrganizationEnv(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if _, err := rt.organizations.GetOrganization(r.Context(), id); errors.Is(err, store.ErrOrganizationNotFound) {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set organization env: load organization failed", slog.String("error", err.Error()), slog.String("id", id))
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

	if err := rt.organizations.SetOrganizationEnvVars(r.Context(), id, vars); err != nil {
		rt.logger.Error("api: set organization env failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, vars)
}
