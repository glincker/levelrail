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

// Package note (projects.go): a project is TASKS.md/repo-plan section
// 6's Phase 4 note made explicit: "Teams, projects, environments, RBAC
// with a small role set. Resist the urge to build a permission matrix."
// This file is deliberately *not* that. There is no owner, no member
// list, no per-project ability of any kind: a project is purely an
// optional label an app or database can be filed under
// (migrations/0022_projects.sql's own comment), and the single admin
// user sees every project and every app/database regardless of
// membership, exactly the way it already sees everything else. No auth
// change of any kind lives in this file; every route below is gated by
// the same requireAbility(AbilityRead/AbilityWrite, ...) shape apps.go
// and databases.go already use, nothing new.
//
// There is deliberately no GET /api/v1/projects/{id}/apps or similar
// server-side filtered listing: the project detail page
// (web/src/routes/projects/$id.tsx) filters the same full
// GET /api/v1/apps and GET /api/v1/databases responses the unfiltered
// list pages already fetch (by project_id, a field toAppResource/
// toDatabaseResource already includes in every row), the same
// "additive organization, not a forced migration" principle that keeps
// project_id nullable rather than reshaping either list endpoint's
// contract. Revisit only if a real scale problem shows up; there is no
// speculative server-side filtering surface built ahead of that.

// projectResource is the wire shape for a project: store.Project as-is,
// the same direct-marshal choice appResource/databaseResource/
// backupTargetResource all make for their own store types.
type projectResource struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

func toProjectResource(p store.Project) projectResource {
	return projectResource{ID: p.ID, Name: p.Name, CreatedAt: p.CreatedAt}
}

// createProjectRequest is POST /api/v1/projects's body: just a name,
// the only field a caller ever chooses (id is minted server-side,
// created_at is server-set).
type createProjectRequest struct {
	Name string `json:"name"`
}

// handleListProjects handles GET /api/v1/projects.
func (rt *Router) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := rt.projects.ListProjects(r.Context())
	if err != nil {
		rt.logger.Error("api: list projects failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]projectResource, 0, len(projects))
	for _, p := range projects {
		out = append(out, toProjectResource(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetProject handles GET /api/v1/projects/{id}, the same
// get-by-id shape every other resource in this router has
// (GET /apps/{name}, GET /databases/{name}, GET /nodes/{id}); the
// project detail page needs this to resolve the project's own name,
// separately from the filtered app/database lists it renders alongside
// it (see this file's own package doc comment for why those stay
// client-side filters of the existing list endpoints, not a new
// server-side filtered route).
func (rt *Router) handleGetProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	p, err := rt.projects.GetProject(r.Context(), id)
	if errors.Is(err, store.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get project failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toProjectResource(p))
}

// handleCreateProject handles POST /api/v1/projects. No conflict check
// against an existing name: migrations/0022_projects.sql's own comment
// explains why project names are deliberately not unique, a project is
// always addressed by id, never by name, so there is no duplicate-name
// case to reject the way handleCreateApp/handleCreateDatabase reject a
// duplicate name (which *is* how those resources are addressed).
func (rt *Router) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	id, err := randomProjectID()
	if err != nil {
		rt.logger.Error("api: create project: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	p := store.Project{
		ID:        id,
		Name:      req.Name,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := rt.projects.SaveProject(r.Context(), p); err != nil {
		rt.logger.Error("api: create project failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, toProjectResource(p))
}

// handleDeleteProject handles DELETE /api/v1/projects/{id}. See
// store.DB.DeleteProject's own doc comment for the behavior this
// deliberately relies on rather than re-implements: desired_services.
// project_id and desired_databases.project_id are
// "ON DELETE SET NULL" foreign keys (migrations/0022_projects.sql), so
// every app and database that belonged to this project is left running
// exactly as it was, simply project-less again. This is not a gap left
// for later, it's the chosen, documented behavior: a project is an
// organizational label, not a lifecycle owner, so deleting the label
// must never delete or disrupt the real resources it labeled.
func (rt *Router) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	err := rt.projects.DeleteProject(r.Context(), id)
	if errors.Is(err, store.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete project failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// randomProjectID mirrors randomBackupTargetID/randomTokenID exactly:
// 9 random bytes, URL-safe base64, a short type prefix. Duplicated
// rather than shared, the same "different resource, different ID
// space, nothing depends on them being interchangeable" reasoning those
// functions' own doc comments already give each other.
func randomProjectID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate project id: %w", err)
	}
	return "proj_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// validateProjectID checks a candidate project_id the same way
// validatePlacementTarget checks a candidate node_id: empty is always
// valid ("no project"), a non-empty value must resolve to a real
// project row, so a typo'd or already-deleted project id fails loudly
// here with a 400 rather than silently being written and never shown
// anywhere (a project's own delete leaves this app/database
// project-less via ON DELETE SET NULL, but a *client* handing this
// endpoint a stale or invented id gets a clear rejection instead).
func (rt *Router) validateProjectID(ctx context.Context, projectID string) error {
	if projectID == "" {
		return nil
	}
	_, err := rt.projects.GetProject(ctx, projectID)
	return err
}
