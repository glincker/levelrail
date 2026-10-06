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

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

// environmentResource is the wire shape for an environment.
type environmentResource struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
	CreatedAt string `json:"created_at"`
	Kind      string `json:"kind"`
	Scope     string `json:"scope"`
	SortOrder int    `json:"sort_order"`
}

func toEnvironmentResource(e store.Environment) environmentResource {
	return environmentResource{ID: e.ID, ProjectID: e.ProjectID, Name: e.Name, Protected: e.Protected, CreatedAt: e.CreatedAt, Kind: e.Kind, Scope: e.Scope, SortOrder: e.SortOrder}
}

type createEnvironmentRequest struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected,omitempty"`
}

type updateEnvironmentRequest struct {
	Protected *bool   `json:"protected"`
	Name      *string `json:"name"`
	Kind      *string `json:"kind"`
	SortOrder *int    `json:"sort_order"`
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

// handleUpdateEnvironment handles PATCH /api/v1/environments/{id}. The
// protected flag is always editable; name, kind and sort_order belong to
// the global-environments feature.
func (rt *Router) handleUpdateEnvironment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req updateEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if (req.Name != nil || req.Kind != nil || req.SortOrder != nil) && !requireExperimentalOn(w, experimental.GlobalEnvironments) {
		return
	}
	current, err := rt.environments.GetEnvironment(r.Context(), id)
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		writeError(w, http.StatusNotFound, "environment not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: update environment: load failed", err, slog.String("id", id))
		return
	}
	if msg := validateEnvironmentPatch(current, req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	err = rt.environments.UpdateEnvironment(r.Context(), id, store.EnvironmentPatch{Name: req.Name, Kind: req.Kind, SortOrder: req.SortOrder, Protected: req.Protected})
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		writeError(w, http.StatusNotFound, "environment not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: update environment failed", err, slog.String("id", id))
		return
	}

	e, err := rt.environments.GetEnvironment(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: update environment: reload failed", err, slog.String("id", id))
		return
	}
	writeJSON(w, http.StatusOK, toEnvironmentResource(e))
}

// handleDeleteEnvironment handles DELETE /api/v1/environments/{id}. A
// global environment is refused while apps or databases are tagged unless
// move_to names where they go; a project one keeps its older behavior
// (tagged services are untagged, or torn down with cascade=true).
func (rt *Router) handleDeleteEnvironment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if reservedEnvironmentIDs[id] {
		writeError(w, http.StatusForbidden, "this environment is built in and cannot be deleted")
		return
	}
	moveTo := r.URL.Query().Get("move_to")
	if moveTo != "" && !rt.moveMembersBeforeDelete(w, r, id, moveTo) {
		return
	}
	if moveTo == "" && !rt.refuseDeleteWithMembers(w, r, id) {
		return
	}
	rt.deleteWithMembers(w, r, "environment", func(ctx context.Context) (cascadeScope, error) {
		return rt.environmentScope(ctx, id)
	}, func(ctx context.Context, _ cascadeScope) error {
		return rt.environments.DeleteEnvironment(ctx, id)
	})
}

type setAppEnvironmentRequest struct {
	EnvironmentID string `json:"environment_id"`
	Confirm       bool   `json:"confirm,omitempty"`
	freezeOverride
}

// handleSetAppEnvironment handles PUT /api/v1/apps/{name}/environment.
// environment_id "" clears the assignment. See moveToEnvironment for the
// protection, freeze and approval rules.
func (rt *Router) handleSetAppEnvironment(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setAppEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: set app environment: load failed", err, slog.String("name", name))
		return
	}
	rt.moveToEnvironment(w, r, environmentMove{
		kind: environmentMoveApp, name: name, currentID: svc.EnvironmentID,
		targetID: req.EnvironmentID, confirm: req.Confirm, freeze: req.freezeOverride,
	})
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

// checkEnvironmentProtection is handleTriggerDeploy's (deploys.go) own
// gate: an app with no environment, or one tagged with an unprotected
// environment, always passes with protected=false. A missing
// environmentID reference degrades to "not protected" rather than
// blocking the caller with a validation error this endpoint isn't
// otherwise responsible for. When environmentID names a protected
// environment and confirm is true, this writes nothing and returns
// protected=true, ok=true: the caller (handleTriggerDeploy) is
// responsible for routing that case into requestDeployApproval
// (deploy_approvals.go) rather than applying the deploy directly.
func (rt *Router) checkEnvironmentProtection(ctx context.Context, w http.ResponseWriter, environmentID string, confirm bool) (env store.Environment, protected, ok bool) {
	if environmentID == "" {
		return store.Environment{}, false, true
	}
	e, err := rt.environments.GetEnvironment(ctx, environmentID)
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		return store.Environment{}, false, true
	}
	if err != nil {
		rt.internalError(w, "api: check environment protection failed", err, slog.String("environment_id", environmentID))
		return store.Environment{}, false, false
	}
	if !e.Protected {
		return e, false, true
	}
	if !confirm {
		writeEnvironmentConfirmationRequired(w, e)
		return store.Environment{}, false, false
	}
	return e, true, true
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
	GetEnvironmentsByIDs(ctx context.Context, ids []string) (map[string]store.Environment, error)
	ListEnvironmentsByProject(ctx context.Context, projectID string) ([]store.Environment, error)
	DeleteEnvironment(ctx context.Context, id string) error
	SetEnvironmentProtected(ctx context.Context, id string, protected bool) error
	UpdateEnvironment(ctx context.Context, id string, patch store.EnvironmentPatch) error
	ListAllEnvironments(ctx context.Context) ([]store.EnvironmentWithCounts, error)
	EnvironmentMemberCounts(ctx context.Context, envID string) (apps, databases int, err error)
	MoveEnvironmentMembers(ctx context.Context, fromID, toID string) error
	EnvironmentOfDatabase(ctx context.Context, databaseName string) (*store.EnvironmentRef, error)
	SetServiceEnvironment(ctx context.Context, serviceName, envID string) error
	// SetEnvironmentEnvVars/ListEnvironmentEnvVars back GET/PUT
	// /api/v1/environments/{id}/env (environment_env.go): shared env vars
	// every service tagged with this environment inherits, sitting
	// between ProjectStore's own project_env_vars tier and a service's
	// own env (internal/reconcile/application's resolveEnv), full-replace
	// on write, mirroring SetOrganizationEnvVars/ListOrganizationEnvVars.
	SetEnvironmentEnvVars(ctx context.Context, environmentID string, vars map[string]string) error
	ListEnvironmentEnvVars(ctx context.Context, environmentID string) (map[string]string, error)
	// ListEnvironmentEnvVarsDetailed/SetEnvironmentSecretEnvVar/
	// DeleteEnvironmentSecretEnvVar/ListEnvironmentSecretEnvKeys back GET
	// /api/v1/environments/{id}/env/all and the secret-var sub-resource
	// routes (environment_env.go), mirroring ProjectStore/
	// OrganizationStore's own secret-capable extension.
	ListEnvironmentEnvVarsDetailed(ctx context.Context, environmentID string) ([]store.SharedEnvVar, error)
	SetEnvironmentSecretEnvVar(ctx context.Context, environmentID, key string) error
	DeleteEnvironmentSecretEnvVar(ctx context.Context, environmentID, key string) error
	ListEnvironmentSecretEnvKeys(ctx context.Context, environmentID string) ([]string, error)
}
