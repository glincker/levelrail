package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/glincker/levelrail/internal/dbauthz"
	"github.com/glincker/levelrail/internal/store"
)

type dbRoute struct{ method, path string }

// explorerRoutes are the gated routes for database name n.
func explorerRoutes(n string) []dbRoute {
	b := "/api/v1/databases/" + n
	return []dbRoute{
		{"GET", b + "/schema"}, {"GET", b + "/tables/public/t/rows"}, {"GET", b + "/tables/public/t/structure"},
		{"POST", b + "/query"}, {"POST", b + "/query/write"}, {"POST", b + "/explain"},
		{"GET", b + "/explorer/capabilities"}, {"GET", b + "/keys"}, {"GET", b + "/key"},
		{"GET", b + "/mongo/collections"}, {"POST", b + "/mongo/find"},
		{"GET", b + "/dynamo/tables"}, {"POST", b + "/dynamo/query"}, {"POST", b + "/dynamo/scan"},
		{"GET", b + "/query-history"}, {"DELETE", b + "/query-history"},
		{"GET", b + "/saved-queries"}, {"POST", b + "/saved-queries"}, {"DELETE", b + "/saved-queries/q1"},
		{"GET", b + "/engine-stats"}, {"GET", b + "/advisor"}, {"GET", b + "/branches/x/schema-diff"},
	}
}

// routeDenied reports a gate refusal; any other status means the handler ran.
func routeDenied(t *testing.T, code int) bool {
	t.Helper()
	if code == http.StatusInternalServerError {
		t.Errorf("route answered 500, the gate or handler failed")
	}
	return code == http.StatusForbidden || code == http.StatusNotFound
}

type realCase struct {
	name     string
	abil     []string
	docs     []string
	envKind  string
	wantRead bool
	wantQry  bool
	wantWrit bool
}

func TestDatabaseAuthzRealEngineMatchesSimulate(t *testing.T) {
	denyRoot := stmtDoc("Deny", `"root"`, `"database:prod"`)
	allowWrite := stmtDoc("Allow", `"db:write"`, `"database:prod"`)
	const prodKind = `"environment-kind:production"`
	root, reader, ci := []string{AbilityRoot}, []string{AbilityRead}, []string{AbilityDeploy}
	cases := []realCase{
		{name: "root, no policy", abil: root, wantRead: true, wantQry: true, wantWrit: true},
		{name: "viewer", abil: reader},
		{name: "ci token narrow", abil: ci},
		{name: "ai agent narrow with template", abil: []string{AbilityRead, AbilityWrite, AbilityDeploy}, docs: []string{templateDoc(t, "ai-operator-nonprod")}},
		{name: "template read-only on root", abil: root, docs: []string{templateDoc(t, "read-only")}, wantRead: true, wantQry: true},
		{name: "template database-read-only on root", abil: root, docs: []string{templateDoc(t, "database-read-only")}, wantRead: true, wantQry: true},
		{name: "template database-operator on root", abil: root, docs: []string{templateDoc(t, "database-operator")}, wantRead: true, wantQry: true},
		{name: "template ai-operator-nonprod on root", abil: root, docs: []string{templateDoc(t, "ai-operator-nonprod")}, wantRead: true, wantQry: true},
		{name: "template project-deployer on root", abil: root, docs: []string{templateDoc(t, "project-deployer")}, wantRead: true, wantQry: true},
		{name: "Allow db:write cannot beat template read-only", abil: root, docs: []string{templateDoc(t, "read-only"), allowWrite}, wantRead: true, wantQry: true},
		{name: "Allow db:write cannot beat template ai-operator", abil: reader, docs: []string{templateDoc(t, "ai-operator-nonprod"), allowWrite}},
		{name: "Deny root beats Allow db:write", abil: reader, docs: []string{denyRoot, allowWrite}},
		{name: "Deny read:sensitive on database:* for root", abil: root, docs: []string{stmtDoc("Deny", `"read:sensitive"`, `"database:*"`)}, wantWrit: true},
		{name: "Allow db:write for read only user", abil: reader, docs: []string{allowWrite}, wantWrit: true},
		{name: "Deny db:read on production env, prod db", abil: root, docs: []string{stmtDoc("Deny", `"db:read"`, prodKind)}, envKind: store.EnvironmentKindProduction, wantWrit: true},
		{name: "Deny db:write on production env, prod db", abil: root, docs: []string{stmtDoc("Deny", `"db:write"`, prodKind)}, envKind: store.EnvironmentKindProduction, wantRead: true, wantQry: true},
		{name: "Deny db:read on production env, dev db", abil: root, docs: []string{stmtDoc("Deny", `"db:read"`, prodKind)}, envKind: store.EnvironmentKindDev, wantRead: true, wantQry: true, wantWrit: true},
		{name: "Deny db:write on production env, dev db", abil: root, docs: []string{stmtDoc("Deny", `"db:write"`, prodKind)}, envKind: store.EnvironmentKindDev, wantRead: true, wantQry: true, wantWrit: true},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, db, _ := newPipelineRouter(t)
			bootstrapTestAdmin(t, db)
			ctx := context.Background()
			seedDatabase(t, db, "prod")
			if tc.envKind != "" {
				if err := db.SaveProject(ctx, store.Project{ID: "proj1", Name: "proj1", CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
					t.Fatal(err)
				}
				if err := db.SaveEnvironment(ctx, store.Environment{ID: "env1", ProjectID: "proj1", Name: "e", Kind: tc.envKind, Scope: store.EnvironmentScopeProject, CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
					t.Fatal(err)
				}
				if err := db.SetDatabaseEnvironment(ctx, "prod", "env1"); err != nil {
					t.Fatal(err)
				}
			}
			tokID := "real-" + fmt.Sprint(i)
			plain := seedMatrixToken(t, db, tokID, tc.abil)
			for _, d := range tc.docs {
				attachDoc(t, db, d, store.PrincipalTypeToken, tokID)
			}
			pols, err := rt.policies.ListPoliciesForPrincipal(ctx, store.PrincipalTypeToken, tokID)
			if err != nil {
				t.Fatal(err)
			}
			p := iamPrincipal{Type: store.PrincipalTypeToken, ID: tokID, Abilities: tc.abil, Active: true}
			for _, c := range []struct {
				action dbauthz.Action
				route  dbRoute
				want   bool
			}{
				{dbauthz.ActionRead, dbRoute{"GET", "/api/v1/databases/prod/schema"}, tc.wantRead},
				{dbauthz.ActionQuery, dbRoute{"POST", "/api/v1/databases/prod/query"}, tc.wantQry},
				{dbauthz.ActionWrite, dbRoute{"POST", "/api/v1/databases/prod/query/write"}, tc.wantWrit},
			} {
				code := matrixDo(rt, c.route.method, c.route.path, bearer(plain))
				if got := !routeDenied(t, code); got != c.want {
					t.Errorf("%s route status %d, allowed=%v want %v", c.action, code, got, c.want)
				}
				sim, err := rt.simulateAccess(ctx, p, pols, string(c.action), "database:prod")
				if err != nil {
					t.Fatal(err)
				}
				if sim.Allowed != c.want {
					t.Errorf("%s simulate allowed=%v want %v (route agrees: %v)", c.action, sim.Allowed, c.want, !routeDenied(t, code))
				}
				if c.action == dbauthz.ActionWrite && c.want && code >= 200 && code < 300 {
					t.Errorf("allowed write with no body must be refused by the handler's own checks, got %d", code)
				}
			}
		})
	}
}

// A team scoped caller on another team's database gets 404 on every route,
// whatever Allow or Deny policies say, and never a 403.
func TestDatabaseAuthzTeamScopedHiddenBeforePolicy(t *testing.T) {
	f := newTeamFixture(t)
	for _, role := range allTeamRoles {
		for _, doc := range []string{
			stmtDoc("Deny", `"*"`, `"database:`+dbB+`"`),
			stmtDoc("Allow", `"db:write"`, `"database:`+dbB+`"`),
		} {
			attachDoc(t, f.db, doc, store.PrincipalTypeUser, f.members[role].ID)
		}
	}
	for _, role := range allTeamRoles {
		for _, r := range explorerRoutes(dbB) {
			code, _ := teamDo(t, f.rt, r.method, r.path, "", func(req *http.Request) { req.AddCookie(f.cookies[role]) })
			if code != http.StatusNotFound { // 404, never 403 or 500
				t.Errorf("%s %s %s = %d, want 404", role, r.method, r.path, code)
			}
		}
	}
}

func TestDatabaseAuthzDenyImplication(t *testing.T) {
	root := []string{AbilityRoot}
	tests := []struct {
		deny   string
		action dbauthz.Action
		want   bool
	}{
		{"db:read", dbauthz.ActionRead, false}, {"db:read", dbauthz.ActionQuery, false}, {"db:read", dbauthz.ActionExport, false},
		{"db:query", dbauthz.ActionRead, true}, {"db:query", dbauthz.ActionQuery, false}, {"db:query", dbauthz.ActionExport, false},
		{"db:export", dbauthz.ActionQuery, true}, {"db:export", dbauthz.ActionExport, false},
		{"db:write", dbauthz.ActionRead, true}, {"db:write", dbauthz.ActionWrite, false},
		{"db:read", dbauthz.ActionWrite, true},
	}
	for _, tt := range tests {
		t.Run(tt.deny+" vs "+string(tt.action), func(t *testing.T) {
			rt, db, _ := newPipelineRouter(t)
			bootstrapTestAdmin(t, db)
			seedDatabase(t, db, "prod")
			attachDoc(t, db, stmtDoc("Deny", `"`+tt.deny+`"`, `"database:prod"`), store.PrincipalTypeToken, "impl")
			seedMatrixToken(t, db, "impl", root)
			pols, err := rt.policies.ListPoliciesForPrincipal(context.Background(), store.PrincipalTypeToken, "impl")
			if err != nil {
				t.Fatal(err)
			}
			v, err := rt.dbEvaluate(context.Background(), store.PrincipalTypeToken, "impl", root, pols, dbauthz.EffectiveAbility(tt.action), tt.action, "database:prod")
			if err != nil || v.Allowed != tt.want {
				t.Fatalf("allowed=%v want %v err=%v", v.Allowed, tt.want, err)
			}
		})
	}
}
