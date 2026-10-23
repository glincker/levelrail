package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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
		rt.internalError(w, "api: list organizations failed", err)
		return
	}
	out := make([]organizationResource, 0, len(orgs))
	for _, o := range orgs {
		if !visibleOrganization(r, o.ID) {
			continue
		}
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
		rt.internalError(w, "api: get organization failed", err, slog.String("id", id))
		return
	}
	writeJSON(w, http.StatusOK, toOrganizationResource(o))
}

// handleCreateOrganization handles POST /api/v1/organizations.
func (rt *Router) handleCreateOrganization(w http.ResponseWriter, r *http.Request) {
	var req createOrganizationRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	id, err := randomOrganizationID()
	if err != nil {
		rt.internalError(w, "api: create organization: generate id failed", err)
		return
	}

	o := store.Organization{ID: id, Name: req.Name, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := rt.organizations.SaveOrganization(r.Context(), o); err != nil {
		rt.internalError(w, "api: create organization failed", err, slog.String("id", id))
		return
	}
	writeJSON(w, http.StatusCreated, toOrganizationResource(o))
}

// handleDeleteOrganization handles DELETE /api/v1/organizations/{id}. By
// default member projects survive (projects.org_id is ON DELETE SET NULL).
// cascade=true tears down every app and database in them first, see
// deleteWithMembers.
func (rt *Router) handleDeleteOrganization(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == store.DefaultTeamID {
		writeError(w, http.StatusConflict, "the default team cannot be deleted")
		return
	}
	if rt.refuseOrphaningDelete(w, r, "organization", id) {
		return
	}
	rt.deleteWithMembers(w, r, "organization", func(ctx context.Context) (cascadeScope, error) {
		return rt.organizationScope(ctx, id)
	}, func(ctx context.Context, sc cascadeScope) error {
		for _, projectID := range sc.projects {
			if err := rt.projects.DeleteProject(ctx, projectID); err != nil && !errors.Is(err, store.ErrProjectNotFound) {
				return fmt.Errorf("delete project %q: %w", projectID, err)
			}
		}
		return rt.organizations.DeleteOrganization(ctx, id)
	})
}

type setProjectOrganizationRequest struct {
	OrgID string `json:"org_id"`
}

// handleSetProjectOrganization handles PUT /api/v1/projects/{id}/organization.
// org_id "" clears the assignment.
func (rt *Router) handleSetProjectOrganization(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")

	var req setProjectOrganizationRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if req.OrgID != "" {
		if _, err := rt.organizations.GetOrganization(r.Context(), req.OrgID); err != nil {
			writeError(w, http.StatusBadRequest, "unknown org_id")
			return
		}
	}
	if !rt.mayMoveProjectBetweenTeams(w, r, projectID, req.OrgID) {
		return
	}

	err := rt.organizations.SetProjectOrganization(r.Context(), projectID, req.OrgID)
	if errors.Is(err, store.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: set project organization failed", err, slog.String("project_id", projectID))
		return
	}

	p, err := rt.projects.GetProject(r.Context(), projectID)
	if err != nil {
		rt.internalError(w, "api: reload project after org assignment failed", err, slog.String("project_id", projectID))
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
	// ListOrganizationEnvVarsDetailed/SetOrganizationSecretEnvVar/
	// DeleteOrganizationSecretEnvVar/ListOrganizationSecretEnvKeys back
	// GET /api/v1/organizations/{id}/env/all and the secret-var
	// sub-resource routes (organization_env.go), mirroring ProjectStore's
	// own secret-capable extension one tier down.
	ListOrganizationEnvVarsDetailed(ctx context.Context, orgID string) ([]store.SharedEnvVar, error)
	SetOrganizationSecretEnvVar(ctx context.Context, orgID, key string) error
	DeleteOrganizationSecretEnvVar(ctx context.Context, orgID, key string) error
	ListOrganizationSecretEnvKeys(ctx context.Context, orgID string) ([]string, error)
}
