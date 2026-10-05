package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// cascadeScope is everything a delete of an environment, project or
// organization takes with it.
type cascadeScope struct {
	services     []store.DesiredService
	databases    []store.DesiredDatabase
	environments []string
	projects     []string
}

// cascadeResponse reports how far a cascading delete got. The container
// itself is deleted last, so when Failed is non-empty it still exists and the
// same request resumes where this one stopped.
type cascadeResponse struct {
	Status           string           `json:"status"`
	DeletedApps      []string         `json:"deleted_apps"`
	DeletedDatabases []string         `json:"deleted_databases"`
	TeardownPending  []string         `json:"teardown_pending,omitempty"`
	Failed           []cascadeFailure `json:"failed,omitempty"`
}

type cascadeFailure struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Error string `json:"error"`
}

const (
	cascadeStatusDeleted = "deleted"
	cascadeStatusPartial = "partial"
)

func (rt *Router) environmentScope(ctx context.Context, envID string) (cascadeScope, error) {
	svcs, err := rt.apps.ListDesiredServicesByEnvironment(ctx, envID)
	if err != nil {
		return cascadeScope{}, fmt.Errorf("list apps in environment: %w", err)
	}
	return cascadeScope{services: svcs, environments: []string{envID}}, nil
}

func (rt *Router) projectScope(ctx context.Context, projectID string) (cascadeScope, error) {
	scope := cascadeScope{projects: []string{projectID}}
	svcs, err := rt.apps.ListDesiredServicesByProject(ctx, projectID)
	if err != nil {
		return scope, fmt.Errorf("list apps in project: %w", err)
	}
	scope.services = svcs
	if scope.databases, err = rt.databases.ListDesiredDatabasesByProject(ctx, projectID); err != nil {
		return scope, fmt.Errorf("list databases in project: %w", err)
	}
	envs, err := rt.environments.ListEnvironmentsByProject(ctx, projectID)
	if err != nil {
		return scope, fmt.Errorf("list environments in project: %w", err)
	}
	for _, e := range envs {
		es, err := rt.environmentScope(ctx, e.ID)
		if err != nil {
			return scope, err
		}
		scope.environments = append(scope.environments, e.ID)
		scope.services = append(scope.services, es.services...)
	}
	return scope, nil
}

func (rt *Router) organizationScope(ctx context.Context, orgID string) (cascadeScope, error) {
	var scope cascadeScope
	projects, err := rt.projects.ListProjects(ctx)
	if err != nil {
		return scope, fmt.Errorf("list projects: %w", err)
	}
	for _, p := range projects {
		if p.OrgID != orgID {
			continue
		}
		ps, err := rt.projectScope(ctx, p.ID)
		if err != nil {
			return scope, err
		}
		scope.services = append(scope.services, ps.services...)
		scope.databases = append(scope.databases, ps.databases...)
		scope.environments = append(scope.environments, ps.environments...)
		scope.projects = append(scope.projects, ps.projects...)
	}
	return scope, nil
}

func (s cascadeScope) deduped() cascadeScope {
	seenSvc := map[string]bool{}
	svcs := s.services[:0:0]
	for _, svc := range s.services {
		if !seenSvc[svc.Name] {
			seenSvc[svc.Name] = true
			svcs = append(svcs, svc)
		}
	}
	seenDB := map[string]bool{}
	dbs := s.databases[:0:0]
	for _, d := range s.databases {
		if !seenDB[d.Name] {
			seenDB[d.Name] = true
			dbs = append(dbs, d)
		}
	}
	s.services, s.databases = svcs, dbs
	return s
}

// tearDownScope deletes every app and database in scope. Apps go first so a
// database in scope that only they used is free to go. A database an app
// outside the scope still uses is kept and reported, never broken.
func (rt *Router) tearDownScope(ctx context.Context, scope cascadeScope) cascadeResponse {
	resp := cascadeResponse{DeletedApps: []string{}, DeletedDatabases: []string{}}
	inScope := make(map[string]bool, len(scope.services))
	for _, svc := range scope.services {
		inScope[svc.Name] = true
	}
	for _, svc := range scope.services {
		teardownErr, err := rt.deleteApp(ctx, svc.Name)
		switch {
		case errors.Is(err, store.ErrServiceNotFound):
			continue
		case err != nil:
			resp.Failed = append(resp.Failed, cascadeFailure{Kind: "app", Name: svc.Name, Error: err.Error()})
			continue
		}
		resp.DeletedApps = append(resp.DeletedApps, svc.Name)
		if teardownErr != nil {
			resp.TeardownPending = append(resp.TeardownPending, svc.Name)
		}
	}
	for _, d := range scope.databases {
		if outside := rt.databaseUsersOutside(ctx, d.Name, inScope); len(outside) > 0 {
			resp.Failed = append(resp.Failed, cascadeFailure{Kind: "database", Name: d.Name, Error: "still used by apps outside this scope: " + strings.Join(outside, ", ")})
			continue
		}
		if err := rt.databases.DeleteDesiredDatabase(ctx, d.Name); err != nil && !errors.Is(err, store.ErrDatabaseNotFound) {
			resp.Failed = append(resp.Failed, cascadeFailure{Kind: "database", Name: d.Name, Error: err.Error()})
			continue
		}
		rt.teardownDatabaseContainer(d.Name, d.NodeID)
		resp.DeletedDatabases = append(resp.DeletedDatabases, d.Name)
	}
	rt.nudgeReconciler()
	resp.Status = cascadeStatusDeleted
	if len(resp.Failed) > 0 {
		resp.Status = cascadeStatusPartial
	}
	return resp
}

func (rt *Router) databaseUsersOutside(ctx context.Context, dbName string, inScope map[string]bool) []string {
	users, err := rt.appsUsingDatabase(ctx, dbName)
	if err != nil {
		return []string{"(could not check: " + err.Error() + ")"}
	}
	var outside []string
	for _, u := range users {
		if !inScope[u] {
			outside = append(outside, u)
		}
	}
	sort.Strings(outside)
	return outside
}

// deleteWithMembers is the shared body of the three container deletes. By
// default members are detached and keep running (the label-only behavior).
// With cascade=true they are torn down first and the container is removed only
// once nothing is left in it, so a partial failure is resumed by repeating the
// request.
func (rt *Router) deleteWithMembers(w http.ResponseWriter, r *http.Request, kind string, scope func(context.Context) (cascadeScope, error), remove func(context.Context, cascadeScope) error) {
	ctx := r.Context()
	cascade := r.URL.Query().Get("cascade") == "true"
	var sc cascadeScope
	if cascade {
		var err error
		if sc, err = scope(ctx); err != nil {
			rt.internalError(w, "api: delete "+kind+": resolve members failed", err)
			return
		}
		sc = sc.deduped()
	}
	resp := rt.tearDownScope(ctx, sc)
	if resp.Status == cascadeStatusPartial {
		rt.logger.Warn("api: cascading delete incomplete", slog.String("kind", kind), slog.Int("failed", len(resp.Failed)))
		writeJSON(w, http.StatusMultiStatus, resp)
		return
	}
	if err := remove(ctx, sc); err != nil {
		if errors.Is(err, store.ErrEnvironmentNotFound) || errors.Is(err, store.ErrProjectNotFound) || errors.Is(err, store.ErrOrganizationNotFound) {
			writeError(w, http.StatusNotFound, kind+" not found")
			return
		}
		rt.internalError(w, "api: delete "+kind+" failed", err)
		return
	}
	if cascade && (len(resp.DeletedApps) > 0 || len(resp.DeletedDatabases) > 0) {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
