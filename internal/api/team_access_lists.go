package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// resolveProjectTeam picks and authorizes the team a new project joins.
func (rt *Router) resolveProjectTeam(w http.ResponseWriter, r *http.Request, team string) (string, bool) {
	scope, _ := callerTeamScope(r.Context())
	if team == "" {
		team = requestScopeValue(r, scopeTeamHeader, scopeTeamCookie)
	}
	if team == "" {
		team = soleWritableTeam(scope)
	}
	if team == "" {
		writeError(w, http.StatusBadRequest, "team_id is required: you belong to several teams")
		return "", false
	}
	if _, err := rt.organizations.GetOrganization(r.Context(), team); err != nil {
		writeError(w, http.StatusBadRequest, "unknown team_id")
		return "", false
	}
	if scope != nil && !store.TeamRoleAtLeast(scope.roleOfTeam(team), store.TeamRoleDeveloper) {
		writeError(w, http.StatusBadRequest, "unknown team_id")
		return "", false
	}
	return team, true
}

func soleWritableTeam(scope *teamScope) string {
	if scope == nil || scope.instance {
		return store.DefaultTeamID
	}
	found := ""
	for id, role := range scope.roles {
		if store.TeamRoleAtLeast(role, store.TeamRoleDeveloper) {
			if found != "" {
				return ""
			}
			found = id
		}
	}
	return found
}

// filterEnvironments hides other teams' project environments and recounts
// shared environments from the caller's own apps and databases only.
func (rt *Router) filterEnvironments(r *http.Request, envs []store.EnvironmentWithCounts) ([]store.EnvironmentWithCounts, error) {
	if requestTeamScope(r) == nil {
		return envs, nil
	}
	_, _, scope, err := rt.callerScope(r)
	if err != nil {
		return nil, err
	}
	db := rt.teamDB()
	if db == nil {
		return envs, nil
	}
	teams, err := db.ProjectTeams(r.Context())
	if err != nil {
		return nil, err
	}
	apps, dbs := map[string]int{}, map[string]int{}
	for name, ref := range scope.appEnv {
		if ref != nil && scope.visible(resourcePrefixApp, name) {
			apps[ref.ID]++
		}
	}
	for name, ref := range scope.dbEnv {
		if ref != nil && scope.visible(resourcePrefixDatabase, name) {
			dbs[ref.ID]++
		}
	}
	out := envs[:0:0]
	for _, e := range envs {
		if e.Scope != store.EnvironmentScopeGlobal && !scope.teams.allowsProject(teams[e.ProjectID], e.ProjectID) {
			continue
		}
		e.AppCount, e.DatabaseCount = apps[e.ID], dbs[e.ID]
		out = append(out, e)
	}
	return out, nil
}
