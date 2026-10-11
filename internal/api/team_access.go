package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Both wrap ErrProjectNotFound so every existing caller answers 400.
var (
	errProjectRequired = fmt.Errorf("project_id is required for a team member: %w", store.ErrProjectNotFound)
	errProjectNotOwned = fmt.Errorf("project is outside your teams: %w", store.ErrProjectNotFound)
)

// requestTeamScope returns the gate's scope for r, narrowed by the switcher.
func requestTeamScope(r *http.Request) *teamScope {
	scope, _ := callerTeamScope(r.Context())
	return narrowTeamScope(r, scope)
}

// authorizeProjectAssignment is the team check behind every write that files
// an app or database under a project: the caller needs write in the project's
// team. A tenant may not leave the project empty, since that files the
// resource under the default team.
func (rt *Router) authorizeProjectAssignment(ctx context.Context, projectID string) error {
	scope, ok := callerTeamScope(ctx)
	if !ok || scope == nil {
		return nil
	}
	db := rt.teamDB()
	if db == nil {
		return nil
	}
	if projectID == "" {
		if scope.instance {
			return nil
		}
		return errProjectRequired
	}
	team, err := db.TeamOfProject(ctx, projectID)
	if errors.Is(err, store.ErrProjectNotFound) {
		return errProjectNotOwned
	}
	if err != nil {
		return err
	}
	role := scope.roleFor(&store.ResourceOwner{TeamID: team, ProjectID: projectID})
	if !store.TeamRoleAtLeast(role, store.TeamRoleDeveloper) {
		return errProjectNotOwned
	}
	return nil
}

// defaultProjectFor fills an empty project_id from the switcher's project.
func defaultProjectFor(r *http.Request, projectID string) string {
	if projectID != "" {
		return projectID
	}
	return requestScopeValue(r, scopeProjectHeader, scopeProjectCookie)
}

// mayMoveProjectBetweenTeams requires admin in the destination team; the
// gate already required write in the source. An empty destination means the
// default team.
func (rt *Router) mayMoveProjectBetweenTeams(w http.ResponseWriter, r *http.Request, projectID, dest string) bool {
	scope, ok := callerTeamScope(r.Context())
	if !ok || scope == nil {
		return true
	}
	if dest == "" {
		dest = store.DefaultTeamID
	}
	src := ""
	if db := rt.teamDB(); db != nil {
		team, err := db.TeamOfProject(r.Context(), projectID)
		if err != nil && !errors.Is(err, store.ErrProjectNotFound) {
			rt.internalError(w, "api: move project: source team", err)
			return false
		}
		src = team
	}
	if !store.TeamRoleAtLeast(scope.roleOfTeam(dest), store.TeamRoleAdmin) || !store.TeamRoleAtLeast(scope.roleOfTeam(src), store.TeamRoleAdmin) {
		writeError(w, http.StatusForbidden, "moving a project between teams needs admin in both teams")
		return false
	}
	return true
}

// refuseOrphaningDelete stops a non-cascading delete that would leave a
// team's apps project-less, which files them under the default team.
func (rt *Router) refuseOrphaningDelete(w http.ResponseWriter, r *http.Request, kind, id string) bool {
	if r.URL.Query().Get("cascade") == "true" {
		return false
	}
	var sc cascadeScope
	var err error
	switch kind {
	case "project":
		db := rt.teamDB()
		if db == nil {
			return false
		}
		team, terr := db.TeamOfProject(r.Context(), id)
		if terr != nil || team == store.DefaultTeamID {
			return false
		}
		sc, err = rt.projectScope(r.Context(), id)
	case "organization":
		sc, err = rt.organizationScope(r.Context(), id)
	}
	if err != nil {
		rt.internalError(w, "api: delete "+kind+": members", err)
		return true
	}
	if len(sc.services) == 0 && len(sc.databases) == 0 && (kind == "project" || len(sc.projects) == 0) {
		return false
	}
	writeError(w, http.StatusConflict, "this "+kind+" still holds apps or databases of a team; delete with cascade=true, or move them first")
	return true
}

// filterProjects keeps the projects the caller's teams own.
func (rt *Router) filterProjects(r *http.Request, projects []store.Project) ([]store.Project, error) {
	scope := requestTeamScope(r)
	if scope == nil {
		return projects, nil
	}
	db := rt.teamDB()
	if db == nil {
		return projects, nil
	}
	teams, err := db.ProjectTeams(r.Context())
	if err != nil {
		return nil, err
	}
	out := projects[:0:0]
	for _, p := range projects {
		if scope.allowsProject(teams[p.ID], p.ID) {
			out = append(out, p)
		}
	}
	return out, nil
}

// visibleOrganization reports whether the caller sees a team.
func visibleOrganization(r *http.Request, id string) bool {
	return requestTeamScope(r).allowsTeam(id)
}
