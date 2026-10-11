package api

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// teamStore is the store surface team scoping needs; *store.DB satisfies it.
type teamStore interface {
	UserTeamRoles(ctx context.Context, userID string) (map[string]string, error)
	GetAPITokenScope(ctx context.Context, tokenID string) (*store.APITokenScope, error)
	OwnerOfApp(ctx context.Context, name string) (*store.ResourceOwner, error)
	OwnerOfDatabase(ctx context.Context, name string) (*store.ResourceOwner, error)
	OwnersOfApps(ctx context.Context) (map[string]*store.ResourceOwner, error)
	OwnersOfDatabases(ctx context.Context) (map[string]*store.ResourceOwner, error)
	TeamOfProject(ctx context.Context, projectID string) (string, error)
	TeamOfEnvironment(ctx context.Context, environmentID string) (store.ResourceOwner, error)
	ProjectTeams(ctx context.Context) (map[string]string, error)
}

func (rt *Router) teamDB() teamStore {
	if s, ok := rt.policies.(teamStore); ok {
		return s
	}
	if s, ok := rt.apps.(teamStore); ok {
		return s
	}
	return nil
}

// Request side narrowing: the dashboard switcher sets cookies, the CLI sends
// headers. Both can only narrow what the caller already sees.
const (
	scopeTeamCookie    = "scope_team"
	scopeProjectCookie = "scope_project"
	scopeTeamHeader    = "X-Scope-Team"
	scopeProjectHeader = "X-Scope-Project"
)

// teamScope limits a principal to the teams it belongs to. A nil scope is
// an instance admin (root) or system token and sees every team.
type teamScope struct {
	roles     map[string]string
	projectID string
	// instance is true for default team members: instance wide routes stay
	// open to them even while a switcher narrows the resource view.
	instance bool
	// allTeams is an instance admin narrowed by project only.
	allTeams bool
}

func (s *teamScope) roleOfTeam(teamID string) string {
	if s.allTeams {
		return store.TeamRoleOwner
	}
	return s.roles[teamID]
}

// roleFor returns the caller's role for a resource owner, or "" when the
// resource is outside the scope. A missing owner means the resource does not
// exist yet, which only instance members may address.
func (s *teamScope) roleFor(o *store.ResourceOwner) string {
	if s == nil {
		return store.TeamRoleOwner
	}
	if o == nil {
		if s.instance {
			return s.roleOfTeam(store.DefaultTeamID)
		}
		return ""
	}
	if s.projectID != "" && o.ProjectID != s.projectID {
		return ""
	}
	return s.roleOfTeam(o.TeamID)
}

// allowsTeam reports whether the scope can see anything of a team.
func (s *teamScope) allowsTeam(teamID string) bool {
	return s == nil || s.roleOfTeam(teamID) != ""
}

// allowsProject reports whether the scope can see a project of a team.
func (s *teamScope) allowsProject(teamID, projectID string) bool {
	if s == nil {
		return true
	}
	if s.projectID != "" && s.projectID != projectID {
		return false
	}
	return s.roleOfTeam(teamID) != ""
}

// capAbilities limits flat abilities to what a team role grants. The default
// team keeps the user's own abilities, which predate teams.
func capAbilities(abilities []string, teamID, role string) []string {
	if role == "" {
		return nil
	}
	if teamID == store.DefaultTeamID && !hasAbility(abilities, AbilityRoot) {
		return abilities
	}
	var out []string
	for _, a := range store.TeamRoleAbilities(role) {
		if hasAbility(abilities, a) {
			out = append(out, a)
		}
	}
	return out
}

// protectedDenied is the environment protection rule: in a protected
// environment, changing anything needs an admin or owner.
func protectedDenied(role, ability string, ref *store.EnvironmentRef) bool {
	if ref == nil || !ref.Protected || store.TeamRoleAtLeast(role, store.TeamRoleAdmin) {
		return false
	}
	return ability == AbilityWrite || ability == AbilityWriteSensitive || ability == AbilityDeploy
}

// principalTeamScope resolves a session user or token to its team scope.
func (rt *Router) principalTeamScope(ctx context.Context, principalType, principalID string) (*teamScope, error) {
	db := rt.teamDB()
	if db == nil {
		return nil, nil
	}
	if principalType != store.PrincipalTypeToken {
		return rt.userTeamScope(ctx, db, principalID)
	}
	rec, err := rt.tokens.GetAPITokenByID(ctx, principalID)
	if err != nil {
		return nil, fmt.Errorf("api: team scope: load token: %w", err)
	}
	var base *teamScope
	if rec.OwnerUserID != "" {
		if base, err = rt.userTeamScope(ctx, db, rec.OwnerUserID); err != nil {
			return nil, err
		}
	}
	limit, err := db.GetAPITokenScope(ctx, principalID)
	if err != nil {
		return nil, fmt.Errorf("api: team scope: %w", err)
	}
	if limit == nil {
		return base, nil
	}
	role := limit.Role
	if base != nil {
		role = store.LowerTeamRole(role, base.roles[limit.TeamID])
		if base.roles[limit.TeamID] == "" {
			role = ""
		}
	}
	out := &teamScope{roles: map[string]string{}, projectID: limit.ProjectID}
	if role != "" {
		out.roles[limit.TeamID] = role
	}
	out.instance = limit.TeamID == store.DefaultTeamID && limit.ProjectID == "" && role != ""
	return out, nil
}

func (rt *Router) userTeamScope(ctx context.Context, db teamStore, userID string) (*teamScope, error) {
	user, err := rt.auth.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("api: team scope: load user: %w", err)
	}
	if slices.Contains(user.Abilities, AbilityRoot) {
		return nil, nil
	}
	roles, err := db.UserTeamRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("api: team scope: %w", err)
	}
	return &teamScope{roles: roles, instance: roles[store.DefaultTeamID] != ""}, nil
}

// narrowTeamScope applies the switcher's team and project to a scope. It
// never widens: an unknown team simply leaves nothing visible.
func narrowTeamScope(r *http.Request, s *teamScope) *teamScope {
	team := requestScopeValue(r, scopeTeamHeader, scopeTeamCookie)
	project := requestScopeValue(r, scopeProjectHeader, scopeProjectCookie)
	if team == "" && project == "" {
		return s
	}
	out := &teamScope{roles: map[string]string{}, projectID: project, instance: s == nil || s.instance}
	if s == nil {
		if team == "" {
			out.allTeams = true
		} else {
			out.roles[team] = store.TeamRoleOwner
		}
		return out
	}
	if s.projectID != "" {
		out.projectID = s.projectID
		if project != "" && project != s.projectID {
			out.roles = map[string]string{}
			return out
		}
	}
	for id, role := range s.roles {
		if team == "" || id == team {
			out.roles[id] = role
		}
	}
	return out
}

func requestScopeValue(r *http.Request, header, cookie string) string {
	if v := strings.TrimSpace(r.Header.Get(header)); v != "" {
		return v
	}
	if c, err := r.Cookie(cookie); err == nil {
		return strings.TrimSpace(c.Value)
	}
	return ""
}

func (rt *Router) ownerForResource(ctx context.Context, prefix, name string) (*store.ResourceOwner, error) {
	db := rt.teamDB()
	if db == nil {
		return nil, nil
	}
	if prefix == resourcePrefixDatabase {
		return db.OwnerOfDatabase(ctx, name)
	}
	return db.OwnerOfApp(ctx, name)
}
