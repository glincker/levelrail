package api

import (
	"fmt"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/resourceadvisor"
	"github.com/GLINCKER/levelrail/internal/store"
)

// scopedAdvisorRefs reports whether an advisor finding's resource sits in the
// picked scope. Nodes belong to no project, so a scoped view drops them.
func (rt *Router) scopedAdvisorRefs(r *http.Request) (func(resourceadvisor.Ref) bool, error) {
	inApp, err := rt.scopedAppVisibility(r)
	if err != nil {
		return nil, err
	}
	dbs, err := rt.databases.ListDesiredDatabases(r.Context())
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	if dbs, err = rt.scopedDatabases(r, dbs); err != nil {
		return nil, fmt.Errorf("scope databases: %w", err)
	}
	inDB := make(map[string]bool, len(dbs))
	for _, d := range dbs {
		inDB[d.Name] = true
	}
	return func(ref resourceadvisor.Ref) bool {
		switch ref.Kind {
		case resourceadvisor.ResourceApp:
			return inApp(ref.Name)
		case resourceadvisor.ResourceDatabase:
			return inDB[ref.Name]
		default:
			return false
		}
	}, nil
}

// requestScoped reports whether the dashboard scope picker params are present.
func requestScoped(r *http.Request) bool {
	q := r.URL.Query()
	return q.Get("project") != "" || len(q["environment"]) > 0
}

// scopedAppVisibility narrows appVisibilityFilter by ?project and
// ?environment, the same filters the apps list takes, so dashboard widgets
// follow the scope picker without a second mechanism.
func (rt *Router) scopedAppVisibility(r *http.Request) (func(app string) bool, error) {
	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		return nil, err
	}
	if !requestScoped(r) {
		return canSee, nil
	}
	fs, ok := rt.apps.(appFilterStore)
	if !ok {
		return nil, errNoFilteredList
	}
	f := parseAppListFilter(r)
	names, err := fs.ListAppNamesFiltered(r.Context(), store.AppListFilter{
		ProjectID:    f.ProjectID,
		Environment:  f.Environment,
		Environments: f.Environments,
	})
	if err != nil {
		return nil, fmt.Errorf("list apps in scope: %w", err)
	}
	inScope := make(map[string]bool, len(names))
	for _, n := range names {
		inScope[n] = true
	}
	return func(app string) bool { return inScope[app] && canSee(app) }, nil
}

// scopedDatabases applies ?project and ?environment to a database list, the
// same filters handleListDatabases uses.
func (rt *Router) scopedDatabases(r *http.Request, dbs []store.DesiredDatabase) ([]store.DesiredDatabase, error) {
	if project := r.URL.Query().Get("project"); project != "" {
		inProject := dbs[:0:0]
		for _, d := range dbs {
			if d.ProjectID == project {
				inProject = append(inProject, d)
			}
		}
		dbs = inProject
	}
	return rt.filterDatabasesByEnvironment(r, dbs)
}
