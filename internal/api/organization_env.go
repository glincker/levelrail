package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// sharedEnvScope parameterizes handleGetSharedEnv/handleSetSharedEnv
// across the organization- and project-level env var endpoints, which
// otherwise differ only in which store method backs each of these three
// operations.
type sharedEnvScope struct {
	label       string
	notFoundMsg string
	notFound    error
	load        func(ctx context.Context, id string) error
	list        func(ctx context.Context, id string) (map[string]string, error)
	set         func(ctx context.Context, id string, vars map[string]string) error
}

func (rt *Router) handleGetSharedEnv(w http.ResponseWriter, r *http.Request, scope sharedEnvScope) {
	id := r.PathValue("id")

	if err := scope.load(r.Context(), id); errors.Is(err, scope.notFound) {
		writeError(w, http.StatusNotFound, scope.notFoundMsg)
		return
	} else if err != nil {
		rt.logger.Error("api: get "+scope.label+" env: load "+scope.label+" failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	vars, err := scope.list(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: get "+scope.label+" env failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, vars)
}

func (rt *Router) handleSetSharedEnv(w http.ResponseWriter, r *http.Request, scope sharedEnvScope) {
	id := r.PathValue("id")

	if err := scope.load(r.Context(), id); errors.Is(err, scope.notFound) {
		writeError(w, http.StatusNotFound, scope.notFoundMsg)
		return
	} else if err != nil {
		rt.logger.Error("api: set "+scope.label+" env: load "+scope.label+" failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	vars, err := decodeEnvVars(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := scope.set(r.Context(), id, vars); err != nil {
		rt.logger.Error("api: set "+scope.label+" env failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, vars)
}

// handleGetOrganizationEnv handles GET /api/v1/organizations/{id}/env.
func (rt *Router) handleGetOrganizationEnv(w http.ResponseWriter, r *http.Request) {
	rt.handleGetSharedEnv(w, r, rt.organizationEnvScope())
}

// handleSetOrganizationEnv handles PUT /api/v1/organizations/{id}/env:
// full replace, mirroring handleSetProjectEnv one tier up.
func (rt *Router) handleSetOrganizationEnv(w http.ResponseWriter, r *http.Request) {
	rt.handleSetSharedEnv(w, r, rt.organizationEnvScope())
}

func (rt *Router) organizationEnvScope() sharedEnvScope {
	return sharedEnvScope{
		label:       "organization",
		notFoundMsg: "organization not found",
		notFound:    store.ErrOrganizationNotFound,
		load: func(ctx context.Context, id string) error {
			_, err := rt.organizations.GetOrganization(ctx, id)
			return err
		},
		list: rt.organizations.ListOrganizationEnvVars,
		set:  rt.organizations.SetOrganizationEnvVars,
	}
}
