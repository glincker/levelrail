package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Audit actions for default environments.
const (
	AuditActionEnvironmentDefaultChanged   = "environment.default.changed"
	AuditActionProjectDefaultEnvCreated    = "project.default_environment.created"
	msgDefaultEnvironmentDeleteRefused     = "this is the project's default environment; make another environment the default first"
	msgDefaultEnvironmentNotInProject      = "environment not found"
	defaultEnvironmentBackfillCreatedLabel = "backfill"
)

// defaultEnvironmentStore is the optional store surface behind project default environments.
type defaultEnvironmentStore interface {
	DefaultEnvironmentID(ctx context.Context, projectID string) (string, error)
	DefaultEnvironmentIDs(ctx context.Context) (map[string]string, error)
	SaveDefaultEnvironment(ctx context.Context, e store.Environment) error
	SetProjectDefaultEnvironment(ctx context.Context, projectID, envID string) error
	ProjectsWithoutDefaultEnvironment(ctx context.Context) ([]store.Project, error)
}

func (rt *Router) defaultEnvStore() (defaultEnvironmentStore, bool) {
	s, ok := rt.environments.(defaultEnvironmentStore)
	return s, ok
}

func (rt *Router) projectDefaultEnvSettings(ctx context.Context) (on bool, name string) {
	return rt.runtimeSettings.Bool(ctx, runtimesettings.KeyProjectsDefaultEnvironment),
		strings.TrimSpace(rt.runtimeSettings.String(ctx, runtimesettings.KeyProjectsDefaultEnvironmentName))
}

func (rt *Router) projectResourceWithDefault(ctx context.Context, p store.Project) projectResource {
	res := toProjectResource(p)
	if s, ok := rt.defaultEnvStore(); ok {
		if id, err := s.DefaultEnvironmentID(ctx, p.ID); err == nil {
			res.DefaultEnvironmentID = id
		}
	}
	return res
}

// createProjectDefaultEnvironment adds the configured default environment to a new project.
func (rt *Router) createProjectDefaultEnvironment(r *http.Request, projectID string) error {
	on, name := rt.projectDefaultEnvSettings(r.Context())
	if !on {
		return nil
	}
	s, ok := rt.defaultEnvStore()
	if !ok {
		return nil
	}
	envID, err := randomEnvironmentID()
	if err != nil {
		return err
	}
	e := store.Environment{ID: envID, ProjectID: projectID, Name: name, Kind: store.EnvironmentKindCustom, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := s.SaveDefaultEnvironment(r.Context(), e); err != nil {
		return err
	}
	rt.auditOrgAction(r, AuditActionProjectDefaultEnvCreated, projectID, map[string]any{"project_id": projectID, "environment_id": envID, "name": name})
	return nil
}

type setDefaultEnvironmentRequest struct {
	EnvironmentID string `json:"environment_id"`
}

// handleSetProjectDefaultEnvironment handles PUT /api/v1/projects/{id}/default-environment.
func (rt *Router) handleSetProjectDefaultEnvironment(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	var req setDefaultEnvironmentRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if req.EnvironmentID == "" {
		writeError(w, http.StatusBadRequest, "environment_id is required")
		return
	}
	project, err := rt.projects.GetProject(r.Context(), projectID)
	if errors.Is(err, store.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: set default environment: load project", err, slog.String("project_id", projectID))
		return
	}
	if !rt.mayAdminProject(w, r, projectID) {
		return
	}
	s, ok := rt.defaultEnvStore()
	if !ok {
		writeError(w, http.StatusNotImplemented, "default environments are not available on this store")
		return
	}
	if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusNotFound, msgDefaultEnvironmentNotInProject), environmentRef(req.EnvironmentID)) {
		return
	}
	previous, _ := s.DefaultEnvironmentID(r.Context(), projectID)
	err = s.SetProjectDefaultEnvironment(r.Context(), projectID, req.EnvironmentID)
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		writeError(w, http.StatusNotFound, msgDefaultEnvironmentNotInProject)
		return
	}
	if err != nil {
		rt.internalError(w, "api: set default environment failed", err, slog.String("project_id", projectID))
		return
	}
	rt.auditOrgAction(r, AuditActionEnvironmentDefaultChanged, projectID, map[string]any{"project_id": projectID, "from": previous, "to": req.EnvironmentID})
	writeJSON(w, http.StatusOK, rt.projectResourceWithDefault(r.Context(), project))
}

// mayAdminProject needs project admin: admin or owner in the project's team. Instance callers pass.
func (rt *Router) mayAdminProject(w http.ResponseWriter, r *http.Request, projectID string) bool {
	scope, ok := callerTeamScope(r.Context())
	if !ok || scope == nil || scope.instance {
		return true
	}
	db := rt.teamDB()
	if db == nil {
		return true
	}
	team, err := db.TeamOfProject(r.Context(), projectID)
	if errors.Is(err, store.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return false
	}
	if err != nil {
		rt.internalError(w, "api: project admin check", err, slog.String("project_id", projectID))
		return false
	}
	if !store.TeamRoleAtLeast(scope.roleOfTeam(team), store.TeamRoleAdmin) {
		writeError(w, http.StatusForbidden, msgTeamRoleTooLow)
		return false
	}
	return true
}

// refuseDefaultEnvironmentDelete answers 409 for a project's default environment.
func (rt *Router) refuseDefaultEnvironmentDelete(w http.ResponseWriter, r *http.Request, id string) bool {
	s, ok := rt.defaultEnvStore()
	if !ok {
		return false
	}
	e, err := rt.environments.GetEnvironment(r.Context(), id)
	if err != nil {
		return false
	}
	if def, err := s.DefaultEnvironmentID(r.Context(), e.ProjectID); err == nil && def == id {
		writeError(w, http.StatusConflict, msgDefaultEnvironmentDeleteRefused)
		return true
	}
	return false
}

type backfillDefaultEnvRequest struct {
	DryRun *bool `json:"dry_run"`
}

type backfillDefaultEnvProject struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Action    string `json:"action"`
}

type backfillDefaultEnvResponse struct {
	DryRun   bool                        `json:"dry_run"`
	Name     string                      `json:"environment_name"`
	Projects []backfillDefaultEnvProject `json:"projects"`
	Applied  int                         `json:"applied"`
}

// handleBackfillDefaultEnvironments handles POST /api/v1/projects/default-environments/backfill.
// It only adds a default where a project has none; an environment already carrying the configured name is promoted.
func (rt *Router) handleBackfillDefaultEnvironments(w http.ResponseWriter, r *http.Request) {
	if !callerIsInstance(r) {
		writeError(w, http.StatusForbidden, msgInstanceOnly)
		return
	}
	var req backfillDefaultEnvRequest
	if r.ContentLength != 0 && !decodeReq(w, r, &req) {
		return
	}
	dryRun := req.DryRun == nil || *req.DryRun
	s, ok := rt.defaultEnvStore()
	if !ok {
		writeError(w, http.StatusNotImplemented, "default environments are not available on this store")
		return
	}
	_, name := rt.projectDefaultEnvSettings(r.Context())
	projects, err := s.ProjectsWithoutDefaultEnvironment(r.Context())
	if err != nil {
		rt.internalError(w, "api: backfill default environments: list", err)
		return
	}
	out := backfillDefaultEnvResponse{DryRun: dryRun, Name: name, Projects: []backfillDefaultEnvProject{}}
	for _, p := range projects {
		envs, err := rt.environments.ListEnvironmentsByProject(r.Context(), p.ID)
		if err != nil {
			rt.internalError(w, "api: backfill default environments: list environments", err, slog.String("project_id", p.ID))
			return
		}
		promote := ""
		for _, e := range envs {
			if e.Scope == store.EnvironmentScopeProject && strings.EqualFold(e.Name, name) {
				promote = e.ID
				break
			}
		}
		action := "create"
		if promote != "" {
			action = "promote"
		}
		if !dryRun {
			if promote != "" {
				err = s.SetProjectDefaultEnvironment(r.Context(), p.ID, promote)
			} else {
				var envID string
				if envID, err = randomEnvironmentID(); err == nil {
					err = s.SaveDefaultEnvironment(r.Context(), store.Environment{ID: envID, ProjectID: p.ID, Name: name, Kind: store.EnvironmentKindCustom, CreatedAt: time.Now().UTC().Format(time.RFC3339)})
					promote = envID
				}
			}
			if err != nil {
				rt.internalError(w, "api: backfill default environments: apply", err, slog.String("project_id", p.ID))
				return
			}
			out.Applied++
			rt.auditOrgAction(r, AuditActionProjectDefaultEnvCreated, p.ID, map[string]any{"project_id": p.ID, "environment_id": promote, "name": name, "source": defaultEnvironmentBackfillCreatedLabel})
		}
		out.Projects = append(out.Projects, backfillDefaultEnvProject{ProjectID: p.ID, Name: p.Name, Action: action})
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) setDatabaseEnvironment(ctx context.Context, name, envID string) error {
	ds, ok := rt.databases.(databaseEnvironmentStore)
	if !ok {
		return errNoDatabaseEnvironmentStore
	}
	return ds.SetDatabaseEnvironment(ctx, name, envID)
}
