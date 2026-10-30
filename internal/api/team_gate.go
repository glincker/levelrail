package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

type teamScopeKey struct{}

// callerTeamScope returns the team scope the gate resolved for this request.
// ok is false outside a gated request, where callers must not assume access.
func callerTeamScope(ctx context.Context) (scope *teamScope, ok bool) {
	scope, ok = ctx.Value(teamScopeKey{}).(*teamScope)
	return scope, ok
}

// tenantRoutes are the instance wide routes a member of only non-default
// teams may call. Each one filters by team itself; everything else answers
// 403 for them, so a new route is closed to tenants until reviewed.
var tenantRoutes = map[string]bool{
	"GET /api/v1/apps":                 true,
	"POST /api/v1/apps":                true,
	"GET /api/v1/apps/resource-usage":  true,
	"GET /api/v1/databases":            true,
	"POST /api/v1/databases":           true,
	"GET /api/v1/deployments":          true,
	"GET /api/v1/deployments/summary":  true,
	"GET /api/v1/deployments/stream":   true,
	"GET /api/v1/deploy-approvals":     true,
	"GET /api/v1/attention/feed":       true,
	"GET /api/v1/traffic/summary":      true,
	"GET /api/v1/usage/summary":        true,
	"GET /api/v1/static-sites":         true,
	"GET /api/v1/domains":              true,
	"GET /api/v1/domains/dns-provider": true,
	"GET /api/v1/projects":             true,
	"POST /api/v1/projects":            true,
	"GET /api/v1/environments":         true,
	"GET /api/v1/organizations":        true,
	"GET /api/v1/teams":                true,
	"GET /api/v1/search":               true,
	"GET /api/v1/search/suggest":       true,
	"GET /api/v1/teams/roles":          true,
	"POST /api/v1/team-invites/accept": true,
	"GET /api/v1/team-invites/{token}": true,
	// Mail resources filter by the owning team and answer 404 outside it.
	"GET /api/v1/mail/domains":                       true,
	"POST /api/v1/mail/domains":                      true,
	"GET /api/v1/mail/mailboxes":                     true,
	"POST /api/v1/mail/mailboxes":                    true,
	"POST /api/v1/mail/mailboxes/{address}/suspend":  true,
	"POST /api/v1/mail/mailboxes/{address}/resume":   true,
	"POST /api/v1/mail/mailboxes/{address}/password": true,
	"DELETE /api/v1/mail/mailboxes/{address}":        true,
	"GET /api/v1/mail/aliases":                       true,
	"POST /api/v1/mail/aliases":                      true,
	"DELETE /api/v1/mail/aliases/{address}":          true,
}

// teamOwnedPrefixes are path prefixes whose {id} names a team owned object;
// the gate resolves the owning team and checks the caller's role there.
var teamOwnedPrefixes = []struct {
	prefix string
	kind   string
}{
	{"/api/v1/projects/{id}", "project"},
	{"/api/v1/environments/{id}", "environment"},
	{"/api/v1/organizations/{id}", "team"},
	{"/api/v1/teams/{id}", "team"},
}

// withTeamGate wraps a gated handler with the team boundary for routes the
// resource gate does not already cover.
func (rt *Router) withTeamGate(required string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principalType, principalID, abilities, err := rt.callerPrincipal(r)
		if err != nil {
			rt.internalError(w, "api: team gate: resolve caller", err)
			return
		}
		scope, err := rt.principalTeamScope(r.Context(), principalType, principalID)
		if err != nil {
			rt.internalError(w, "api: team gate: resolve scope", err)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), teamScopeKey{}, scope))
		if scope == nil {
			next(w, r)
			return
		}
		status, msg, err := rt.teamRouteDecision(r, scope, required, abilities)
		if err != nil {
			rt.internalError(w, "api: team gate: decide", err)
			return
		}
		if status != 0 {
			rt.logger.Info("api: team gate denied", slog.String("pattern", r.Pattern), slog.String("principal_id", principalID), slog.Int("status", status))
			writeError(w, status, msg)
			return
		}
		next(w, r)
	}
}

const (
	msgInstanceOnly   = "this is an instance wide action; ask an instance admin, or switch to a team you can manage"
	msgTeamRoleTooLow = "your team role does not allow this; ask a team admin or owner"
)

// teamRouteDecision returns 0 to allow, or the status and message to deny.
func (rt *Router) teamRouteDecision(r *http.Request, scope *teamScope, required string, abilities []string) (int, string, error) {
	_, path, _ := strings.Cut(r.Pattern, " ")
	if strings.HasPrefix(path, "/api/v1/apps/{name}") || strings.HasPrefix(path, "/api/v1/databases/{name}") {
		return 0, "", nil
	}
	if strings.HasPrefix(path, "/api/v1/auth/") {
		return 0, "", nil
	}
	if strings.HasPrefix(path, "/api/v1/deploy-approvals/{id}") {
		return rt.approvalDecision(r, scope, required, abilities)
	}
	for _, p := range teamOwnedPrefixes {
		if path != p.prefix && !strings.HasPrefix(path, p.prefix+"/") {
			continue
		}
		if path == "/api/v1/teams/{id}/members/me" {
			return 0, "", nil
		}
		owner, err := rt.ownerOfObject(r.Context(), p.kind, r.PathValue("id"))
		if errors.Is(err, store.ErrProjectNotFound) || errors.Is(err, store.ErrEnvironmentNotFound) {
			return http.StatusNotFound, p.kind + " not found", nil
		}
		if err != nil {
			return 0, "", err
		}
		role := scope.roleFor(&owner)
		if p.kind == "team" {
			role = scope.roleOfTeam(owner.TeamID)
		}
		if role == "" {
			return http.StatusNotFound, p.kind + " not found", nil
		}
		if !hasAbility(capAbilities(abilities, owner.TeamID, role), required) {
			return http.StatusForbidden, msgTeamRoleTooLow, nil
		}
		if p.kind == "team" && r.Method == http.MethodDelete && path == p.prefix && role != store.TeamRoleOwner {
			return http.StatusForbidden, "only a team owner can delete the team", nil
		}
		return 0, "", nil
	}
	if scope.instance || tenantRoutes[r.Pattern] {
		return 0, "", nil
	}
	return http.StatusForbidden, msgInstanceOnly, nil
}

type approvalStore interface {
	GetDeployApproval(ctx context.Context, id string) (store.DeployApproval, error)
}

// approvalDecision authorizes a deploy approval by the apps it changes and
// reads. Approving is the protected environment gate, so it needs admin.
func (rt *Router) approvalDecision(r *http.Request, scope *teamScope, required string, abilities []string) (int, string, error) {
	db, ok := rt.teamDB().(approvalStore)
	if !ok {
		return http.StatusForbidden, msgInstanceOnly, nil
	}
	a, err := db.GetDeployApproval(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrDeployApprovalNotFound) {
		return http.StatusNotFound, "approval not found", nil
	}
	if err != nil {
		return 0, "", err
	}
	for _, app := range []string{a.ServiceName, a.SourceServiceName} {
		if app == "" {
			continue
		}
		owner, err := rt.ownerForResource(r.Context(), resourcePrefixApp, app)
		if err != nil {
			return 0, "", err
		}
		if owner == nil && !scope.instance {
			return http.StatusNotFound, "approval not found", nil
		}
		role := scope.roleFor(owner)
		if role == "" {
			return http.StatusNotFound, "approval not found", nil
		}
		if !hasAbility(capAbilities(abilities, ownerTeam(owner), role), required) {
			return http.StatusForbidden, msgTeamRoleTooLow, nil
		}
		if required != AbilityRead && app == a.ServiceName && !store.TeamRoleAtLeast(role, store.TeamRoleAdmin) {
			return http.StatusForbidden, "deciding a protected environment deploy needs the admin or owner role", nil
		}
	}
	return 0, "", nil
}

// instanceMembersOnly closes a session only route to tenants. Used where a
// feature fans instance wide data out to whoever enrolls, like web push.
func (rt *Router) instanceMembersOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := rt.currentSessionUserID(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		scope, err := rt.principalTeamScope(r.Context(), store.PrincipalTypeUser, userID)
		if err != nil {
			rt.internalError(w, "api: instance members only", err)
			return
		}
		if scope != nil && !scope.instance {
			writeError(w, http.StatusForbidden, msgInstanceOnly)
			return
		}
		next(w, r)
	}
}

func (rt *Router) ownerOfObject(ctx context.Context, kind, id string) (store.ResourceOwner, error) {
	db := rt.teamDB()
	if db == nil {
		return store.ResourceOwner{TeamID: store.DefaultTeamID}, nil
	}
	switch kind {
	case "project":
		team, err := db.TeamOfProject(ctx, id)
		return store.ResourceOwner{TeamID: team, ProjectID: id}, err
	case "environment":
		return db.TeamOfEnvironment(ctx, id)
	}
	return store.ResourceOwner{TeamID: id}, nil
}
