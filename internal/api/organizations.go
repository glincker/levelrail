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

// organizationResource is the wire shape for an organization.
type organizationResource struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

func toOrganizationResource(o store.Organization) organizationResource {
	return organizationResource{ID: o.ID, Name: o.Name, CreatedAt: o.CreatedAt}
}

type createOrganizationRequest struct {
	Name string `json:"name"`
}

// handleListOrganizations handles GET /api/v1/organizations.
func (rt *Router) handleListOrganizations(w http.ResponseWriter, r *http.Request) {
	orgs, err := rt.organizations.ListOrganizations(r.Context())
	if err != nil {
		rt.logger.Error("api: list organizations failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]organizationResource, 0, len(orgs))
	for _, o := range orgs {
		out = append(out, toOrganizationResource(o))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetOrganization handles GET /api/v1/organizations/{id}.
func (rt *Router) handleGetOrganization(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	o, err := rt.organizations.GetOrganization(r.Context(), id)
	if errors.Is(err, store.ErrOrganizationNotFound) {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get organization failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toOrganizationResource(o))
}

// handleCreateOrganization handles POST /api/v1/organizations.
func (rt *Router) handleCreateOrganization(w http.ResponseWriter, r *http.Request) {
	var req createOrganizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	id, err := randomOrganizationID()
	if err != nil {
		rt.logger.Error("api: create organization: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	o := store.Organization{ID: id, Name: req.Name, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := rt.organizations.SaveOrganization(r.Context(), o); err != nil {
		rt.logger.Error("api: create organization failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, toOrganizationResource(o))
}

// handleDeleteOrganization handles DELETE /api/v1/organizations/{id}.
// projects.org_id is ON DELETE SET NULL, so member projects survive.
func (rt *Router) handleDeleteOrganization(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := rt.organizations.DeleteOrganization(r.Context(), id)
	if errors.Is(err, store.ErrOrganizationNotFound) {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete organization failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type setProjectOrganizationRequest struct {
	OrgID string `json:"org_id"`
}

// handleSetProjectOrganization handles PUT /api/v1/projects/{id}/organization.
// org_id "" clears the assignment.
func (rt *Router) handleSetProjectOrganization(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")

	var req setProjectOrganizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.OrgID != "" {
		if _, err := rt.organizations.GetOrganization(r.Context(), req.OrgID); err != nil {
			writeError(w, http.StatusBadRequest, "unknown org_id")
			return
		}
	}

	err := rt.organizations.SetProjectOrganization(r.Context(), projectID, req.OrgID)
	if errors.Is(err, store.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: set project organization failed", slog.String("error", err.Error()), slog.String("project_id", projectID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	p, err := rt.projects.GetProject(r.Context(), projectID)
	if err != nil {
		rt.logger.Error("api: reload project after org assignment failed", slog.String("error", err.Error()), slog.String("project_id", projectID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toProjectResource(p))
}

// randomOrganizationID mirrors randomProjectID exactly.
func randomOrganizationID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate organization id: %w", err)
	}
	return "org_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// OrganizationStore is the store surface the organizations handlers need.
type OrganizationStore interface {
	SaveOrganization(ctx context.Context, o store.Organization) error
	GetOrganization(ctx context.Context, id string) (store.Organization, error)
	ListOrganizations(ctx context.Context) ([]store.Organization, error)
	DeleteOrganization(ctx context.Context, id string) error
	SetProjectOrganization(ctx context.Context, projectID, orgID string) error
	// SetOrganizationEnvVars/ListOrganizationEnvVars back GET/PUT
	// /api/v1/organizations/{id}/env (organization_env.go): shared env
	// vars every project filed under this organization inherits as the
	// base layer beneath its own project_env_vars tier
	// (internal/reconcile/application's resolveEnv), full-replace on
	// write, mirroring ProjectStore's SetProjectEnvVars/ListProjectEnvVars
	// one level up.
	SetOrganizationEnvVars(ctx context.Context, orgID string, vars map[string]string) error
	ListOrganizationEnvVars(ctx context.Context, orgID string) (map[string]string, error)
}
