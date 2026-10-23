package api

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// teamFixture is two isolated teams, A and B, each with a project, an
// environment, an app and a database, plus one tenant per role in team A.
type teamFixture struct {
	rt      *Router
	db      *store.DB
	members map[string]store.User
	cookies map[string]*http.Cookie
}

const (
	teamA, teamB       = "org_team_a", "org_team_b"
	projA, projB       = "proj_team_a", "proj_team_b"
	envA, envB         = "env_team_a", "env_team_b"
	appA, appB         = "a-web", "b-web"
	dbA, dbB           = "a-db", "b-db"
	projA2, appA2      = "proj_team_a2", "a2-web"
	teamMatrixPassword = "team-matrix-password-1"
	approvalA          = "appr_team_a"
	approvalB          = "appr_team_b"
)

var allTeamRoles = []string{store.TeamRoleOwner, store.TeamRoleAdmin, store.TeamRoleDeveloper, store.TeamRoleViewer}

func newTeamFixture(t *testing.T) *teamFixture {
	t.Helper()
	rt, db, _ := newPipelineRouter(t)
	bootstrapTestAdmin(t, db)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []struct{ team, proj, env, app, database string }{
		{teamA, projA, envA, appA, dbA},
		{teamB, projB, envB, appB, dbB},
	} {
		must(db.SaveOrganization(ctx, store.Organization{ID: s.team, Name: s.team, CreatedAt: "2026-10-01T00:00:00Z"}))
		must(db.SaveProject(ctx, store.Project{ID: s.proj, Name: s.proj, CreatedAt: "2026-10-01T00:00:00Z"}))
		must(db.SetProjectOrganization(ctx, s.proj, s.team))
		must(db.SaveEnvironment(ctx, store.Environment{ID: s.env, ProjectID: s.proj, Name: "prod", CreatedAt: "2026-10-01T00:00:00Z"}))
		must(db.SaveDesiredService(ctx, store.DesiredService{Name: s.app, Image: "nginx:1", Port: 80}))
		must(db.UpdateServiceProject(ctx, s.app, s.proj))
		must(db.SetServiceEnvironment(ctx, s.app, s.env))
		must(db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: s.database, Engine: store.EnginePostgres, Version: "16"}))
		must(db.UpdateDatabaseProject(ctx, s.database, s.proj))
	}
	for id, app := range map[string]string{approvalA: appA, approvalB: appB} {
		must(db.SaveDeployApproval(ctx, store.DeployApproval{ID: id, ServiceName: app, Action: "deploy", Image: "nginx:2", Status: store.DeployApprovalStatusPending,
			CreatedAt: "2026-10-01T00:00:00Z", ExpiresAt: "2099-01-01T00:00:00Z"}))
	}
	must(db.SaveProject(ctx, store.Project{ID: projA2, Name: projA2, CreatedAt: "2026-10-01T00:00:00Z"}))
	must(db.SetProjectOrganization(ctx, projA2, teamA))
	must(db.SaveDesiredService(ctx, store.DesiredService{Name: appA2, Image: "nginx:1", Port: 80}))
	must(db.UpdateServiceProject(ctx, appA2, projA2))

	f := &teamFixture{rt: rt, db: db, members: map[string]store.User{}, cookies: map[string]*http.Cookie{}}
	for _, role := range allTeamRoles {
		u, err := rt.createLocalUserInTeam(ctx, role+"@team-a.test", "", teamMatrixPassword, store.TeamRoleAbilities(role), teamA, role)
		must(err)
		f.members[role] = u
		f.cookies[role] = sessionCookieForTest(t, rt, u.ID)
	}
	return f
}

// otherTeamPath fills a pattern so every id names one of team B's objects.
func otherTeamPath(pattern string) string {
	switch {
	case strings.HasPrefix(pattern, "/api/v1/apps/{name}"):
		return concretePath(pattern, appB)
	case strings.HasPrefix(pattern, "/api/v1/databases/{name}"):
		return concretePath(pattern, dbB)
	case strings.HasPrefix(pattern, "/api/v1/projects/{id}"):
		return strings.Replace(concretePath(pattern, "x"), "/projects/x1", "/projects/"+projB, 1)
	case strings.HasPrefix(pattern, "/api/v1/environments/{id}"):
		return strings.Replace(concretePath(pattern, "x"), "/environments/x1", "/environments/"+envB, 1)
	case strings.HasPrefix(pattern, "/api/v1/organizations/{id}"):
		return strings.Replace(concretePath(pattern, "x"), "/organizations/x1", "/organizations/"+teamB, 1)
	case strings.HasPrefix(pattern, "/api/v1/deploy-approvals/{id}"):
		return strings.Replace(concretePath(pattern, "x"), "/deploy-approvals/x1", "/deploy-approvals/"+approvalB, 1)
	case strings.HasPrefix(pattern, "/api/v1/teams/{id}"):
		return strings.Replace(concretePath(pattern, "x"), "/teams/x1", "/teams/"+teamB, 1)
	}
	return ""
}

// tenantSessionOnly are routes outside the team gate (requireAuth or public)
// a tenant may reach; each must only touch the caller's own records.
var tenantSessionOnly = map[string]string{
	"GET /api/v1/settings/push-subscriptions":                  "lists only the caller's own subscriptions",
	"GET /api/v1/settings/push-subscriptions/vapid-public-key": "public key, no data",
	"DELETE /api/v1/settings/push-subscriptions/{id}":          "deletes only the caller's own subscription",
	"POST /api/v1/attention/dismiss":                           "stores the caller's own dismissal of an item it can already see",
}

func routeIsTenantOpen(key, path string) bool {
	if tenantRoutes[key] || strings.HasPrefix(path, "/api/v1/auth/") {
		return true
	}
	return strings.HasPrefix(path, "/api/v1/teams/{id}") || strings.HasPrefix(path, "/api/v1/team-invites/")
}

// TestTeamAuthzMatrix_OtherTeamIsInvisible drives every team owned route as
// every role of team A against team B's objects: none may answer anything
// but 403 or 404, so nothing of team B can be read or changed.
func TestTeamAuthzMatrix_OtherTeamIsInvisible(t *testing.T) {
	t.Parallel()
	f := newTeamFixture(t)
	routes := loadMatrixRoutes(t)
	for _, role := range allTeamRoles {
		t.Run(role, func(t *testing.T) {
			var bad []string
			checked := 0
			for _, r := range routes {
				path := otherTeamPath(r.pattern)
				if path == "" || r.key() == "DELETE /api/v1/teams/{id}/members/me" {
					continue
				}
				if _, public := publicRoutes[r.key()]; public {
					continue
				}
				if _, lib := libraryOnlyRoutes[r.key()]; lib {
					continue
				}
				checked++
				got := matrixDo(f.rt, r.method, path, func(req *http.Request) { req.AddCookie(f.cookies[role]) })
				if got != http.StatusNotFound && got != http.StatusForbidden && got != http.StatusUnauthorized {
					bad = append(bad, r.key()+" -> "+http.StatusText(got))
				}
			}
			if checked < 100 {
				t.Fatalf("only %d team owned routes checked, the path mapping drifted", checked)
			}
			if len(bad) > 0 {
				sort.Strings(bad)
				t.Errorf("%s of team A reached team B through:\n%s", role, strings.Join(bad, "\n"))
			}
		})
	}
}

// TestTeamAuthzMatrix_TenantInstanceRoutesClosed checks every route that is
// neither team owned nor reviewed for tenants answers a tenant with 403.
func TestTeamAuthzMatrix_TenantInstanceRoutesClosed(t *testing.T) {
	t.Parallel()
	f := newTeamFixture(t)
	var bad []string
	for _, r := range loadMatrixRoutes(t) {
		_, path, _ := strings.Cut(r.key(), " ")
		if otherTeamPath(r.pattern) != "" || routeIsTenantOpen(r.key(), path) {
			continue
		}
		if _, public := publicRoutes[r.key()]; public {
			continue
		}
		if _, lib := libraryOnlyRoutes[r.key()]; lib {
			continue
		}
		if _, ok := tenantSessionOnly[r.key()]; ok {
			continue
		}
		got := matrixDo(f.rt, r.method, concretePath(r.pattern, "web"), func(req *http.Request) { req.AddCookie(f.cookies[store.TeamRoleOwner]) })
		if got != http.StatusForbidden && got != http.StatusUnauthorized {
			bad = append(bad, r.key()+" -> "+http.StatusText(got))
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		t.Errorf("instance routes a team owner reaches (gate it, or review and list it in tenantRoutes or tenantSessionOnly):\n%s", strings.Join(bad, "\n"))
	}
}

// TestTeamAuthzMatrix_ViewerCannotMutateOwnTeam: a viewer of team A changes
// nothing of team A either.
func TestTeamAuthzMatrix_ViewerCannotMutateOwnTeam(t *testing.T) {
	f := newTeamFixture(t)
	var bad []string
	for _, r := range loadMatrixRoutes(t) {
		switch r.method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			continue
		}
		path := otherTeamPath(r.pattern)
		if path == "" {
			continue
		}
		for _, swap := range [][2]string{{appB, appA}, {dbB, dbA}, {projB, projA}, {envB, envA}, {teamB, teamA}, {approvalB, approvalA}} {
			path = strings.Replace(path, swap[0], swap[1], 1)
		}
		if _, ok := readOnlyMayMutate[r.key()]; ok {
			continue
		}
		if r.key() == "DELETE /api/v1/teams/{id}/members/me" {
			continue
		}
		got := matrixDo(f.rt, r.method, path, func(req *http.Request) { req.AddCookie(f.cookies[store.TeamRoleViewer]) })
		if got != http.StatusForbidden && got != http.StatusUnauthorized {
			bad = append(bad, r.key()+" -> "+http.StatusText(got))
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		t.Errorf("mutating routes a team viewer reaches on its own team:\n%s", strings.Join(bad, "\n"))
	}
}

func TestTeamAuthzMatrix_OwnTeamReadable(t *testing.T) {
	f := newTeamFixture(t)
	for _, role := range allTeamRoles {
		for _, path := range []string{"/api/v1/apps/" + appA, "/api/v1/databases/" + dbA, "/api/v1/projects/" + projA, "/api/v1/teams/" + teamA} {
			if got := matrixDo(f.rt, http.MethodGet, path, func(req *http.Request) { req.AddCookie(f.cookies[role]) }); got != http.StatusOK {
				t.Errorf("%s GET %s = %d, want 200", role, path, got)
			}
		}
	}
}
