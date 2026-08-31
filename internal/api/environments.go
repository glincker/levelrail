package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// environmentResource is the wire shape for an environment.
type environmentResource struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
	CreatedAt string `json:"created_at"`
}

func toEnvironmentResource(e store.Environment) environmentResource {
	return environmentResource{ID: e.ID, ProjectID: e.ProjectID, Name: e.Name, Protected: e.Protected, CreatedAt: e.CreatedAt}
}

type createEnvironmentRequest struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected,omitempty"`
}

type updateEnvironmentRequest struct {
	Protected bool `json:"protected"`
}

// handleListEnvironments handles GET /api/v1/projects/{id}/environments.
func (rt *Router) handleListEnvironments(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if _, err := rt.projects.GetProject(r.Context(), projectID); errors.Is(err, store.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	envs, err := rt.environments.ListEnvironmentsByProject(r.Context(), projectID)
	if err != nil {
		rt.logger.Error("api: list environments failed", slog.String("error", err.Error()), slog.String("project_id", projectID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]environmentResource, 0, len(envs))
	for _, e := range envs {
		out = append(out, toEnvironmentResource(e))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateEnvironment handles POST /api/v1/projects/{id}/environments.
func (rt *Router) handleCreateEnvironment(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if _, err := rt.projects.GetProject(r.Context(), projectID); errors.Is(err, store.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	var req createEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	id, err := randomEnvironmentID()
	if err != nil {
		rt.logger.Error("api: create environment: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	e := store.Environment{ID: id, ProjectID: projectID, Name: req.Name, Protected: req.Protected, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := rt.environments.SaveEnvironment(r.Context(), e); err != nil {
		rt.logger.Error("api: create environment failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, toEnvironmentResource(e))
}

// handleUpdateEnvironment handles PATCH /api/v1/environments/{id}: today
// this only ever changes protected, since name/project have no other
// caller-facing edit path either (an environment is otherwise
// create-or-delete, matching how a project's own name is immutable
// too).
func (rt *Router) handleUpdateEnvironment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req updateEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	err := rt.environments.SetEnvironmentProtected(r.Context(), id, req.Protected)
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		writeError(w, http.StatusNotFound, "environment not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: update environment failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	e, err := rt.environments.GetEnvironment(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: update environment: reload failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toEnvironmentResource(e))
}

// handleDeleteEnvironment handles DELETE /api/v1/environments/{id}.
// desired_services.environment_id is ON DELETE SET NULL, so tagged
// services survive, simply untagged again.
func (rt *Router) handleDeleteEnvironment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := rt.environments.DeleteEnvironment(r.Context(), id)
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		writeError(w, http.StatusNotFound, "environment not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete environment failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type setAppEnvironmentRequest struct {
	EnvironmentID string `json:"environment_id"`
}

// handleSetAppEnvironment handles PUT /api/v1/apps/{name}/environment.
// environment_id "" clears the assignment.
func (rt *Router) handleSetAppEnvironment(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setAppEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.EnvironmentID != "" {
		if _, err := rt.environments.GetEnvironment(r.Context(), req.EnvironmentID); err != nil {
			writeError(w, http.StatusBadRequest, "unknown environment_id")
			return
		}
	}

	err := rt.environments.SetServiceEnvironment(r.Context(), name, req.EnvironmentID)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: set app environment failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.reloadAndWriteApp(w, r, name, "set app environment")
}

// environmentNeedsConfirmation reports whether env is protected and
// confirm wasn't already given: the pure predicate behind
// requireEnvironmentConfirmation below and handlePromoteApp's own
// already-loaded res.env check (promote.go), since promotion targets an
// environment it has loaded before this package would otherwise reload
// it by ID.
func environmentNeedsConfirmation(env store.Environment, confirm bool) bool {
	return env.Protected && !confirm
}

func writeEnvironmentConfirmationRequired(w http.ResponseWriter, env store.Environment) {
	writeError(w, http.StatusConflict, fmt.Sprintf("environment %q is protected; set confirm: true to proceed", env.Name))
}

// requireEnvironmentConfirmation is handleTriggerDeploy's (deploys.go)
// own gate: an app with no environment, or one tagged with an
// unprotected environment, always passes. A missing environmentID
// reference degrades to "not protected" rather than blocking the caller
// with a validation error this endpoint isn't otherwise responsible for.
func (rt *Router) requireEnvironmentConfirmation(ctx context.Context, w http.ResponseWriter, environmentID string, confirm bool) bool {
	if environmentID == "" {
		return true
	}
	env, err := rt.environments.GetEnvironment(ctx, environmentID)
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		return true
	}
	if err != nil {
		rt.internalError(w, "api: check environment protection failed", err, slog.String("environment_id", environmentID))
		return false
	}
	if environmentNeedsConfirmation(env, confirm) {
		writeEnvironmentConfirmationRequired(w, env)
		return false
	}
	return true
}

// randomEnvironmentID mirrors randomProjectID exactly.
func randomEnvironmentID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate environment id: %w", err)
	}
	return "env_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// EnvironmentStore is the store surface the environments handlers need.
type EnvironmentStore interface {
	SaveEnvironment(ctx context.Context, e store.Environment) error
	GetEnvironment(ctx context.Context, id string) (store.Environment, error)
	ListEnvironmentsByProject(ctx context.Context, projectID string) ([]store.Environment, error)
	DeleteEnvironment(ctx context.Context, id string) error
	SetEnvironmentProtected(ctx context.Context, id string, protected bool) error
	SetServiceEnvironment(ctx context.Context, serviceName, envID string) error
	// SetEnvironmentEnvVars/ListEnvironmentEnvVars back GET/PUT
	// /api/v1/environments/{id}/env (environment_env.go): shared env vars
	// every service tagged with this environment inherits, sitting
	// between ProjectStore's own project_env_vars tier and a service's
	// own env (internal/reconcile/application's resolveEnv), full-replace
	// on write, mirroring SetOrganizationEnvVars/ListOrganizationEnvVars.
	SetEnvironmentEnvVars(ctx context.Context, environmentID string, vars map[string]string) error
	ListEnvironmentEnvVars(ctx context.Context, environmentID string) (map[string]string, error)
}
