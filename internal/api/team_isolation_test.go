package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func teamDo(t *testing.T, rt *Router, method, path, body string, mutate func(*http.Request)) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequestWithContext(ctx, method, path, rdr)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func withCookie(c *http.Cookie) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(c) }
}

// TestTeamIsolation_ListsAndCounts checks every list a tenant can reach shows
// its own team's objects and never team B's names.
func TestTeamIsolation_ListsAndCounts(t *testing.T) {
	f := newTeamFixture(t)
	lists := []string{
		"/api/v1/apps", "/api/v1/databases", "/api/v1/projects", "/api/v1/environments",
		"/api/v1/organizations", "/api/v1/teams", "/api/v1/deployments", "/api/v1/deploy-approvals?status=all",
		"/api/v1/attention/feed", "/api/v1/static-sites", "/api/v1/domains",
		"/api/v1/usage/summary", "/api/v1/traffic/summary", "/api/v1/auth/tokens", "/api/v1/deployments/summary",
	}
	for _, role := range allTeamRoles {
		for _, path := range lists {
			code, body := teamDo(t, f.rt, http.MethodGet, path, "", withCookie(f.cookies[role]))
			if code != http.StatusOK {
				t.Errorf("%s GET %s = %d %s", role, path, code, body)
				continue
			}
			for _, leak := range []string{appB, dbB, projB, envB, teamB} {
				if strings.Contains(body, leak) {
					t.Errorf("%s GET %s leaks %q: %s", role, path, leak, body)
				}
			}
		}
		_, apps := teamDo(t, f.rt, http.MethodGet, "/api/v1/apps", "", withCookie(f.cookies[role]))
		if !strings.Contains(apps, appA) {
			t.Errorf("%s does not see its own app: %s", role, apps)
		}
	}
}

// TestTeamIsolation_InstanceMemberCannotSeeOtherTeam: a non-root default team
// member keeps instance routes but still cannot read team B.
func TestTeamIsolation_InstanceMemberCannotSeeOtherTeam(t *testing.T) {
	f := newTeamFixture(t)
	op := storeUserWithAbilitiesForTest(t, f.db, "operator@instance.test", []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityDeploy})
	cookie := sessionCookieForTest(t, f.rt, op.ID)
	for _, path := range []string{"/api/v1/apps/" + appB, "/api/v1/databases/" + dbB, "/api/v1/projects/" + projB, "/api/v1/teams/" + teamB} {
		if code, _ := teamDo(t, f.rt, http.MethodGet, path, "", withCookie(cookie)); code != http.StatusNotFound {
			t.Errorf("instance member GET %s = %d, want 404", path, code)
		}
	}
	_, body := teamDo(t, f.rt, http.MethodGet, "/api/v1/apps", "", withCookie(cookie))
	if strings.Contains(body, appB) || strings.Contains(body, appA) {
		t.Errorf("instance member sees team apps: %s", body)
	}
	if code, _ := teamDo(t, f.rt, http.MethodGet, "/api/v1/tags", "", withCookie(cookie)); code != http.StatusOK {
		t.Errorf("instance member lost an instance route: %d", code)
	}
	admin := loginTestSession(t, f.rt, f.db)
	_, all := teamDo(t, f.rt, http.MethodGet, "/api/v1/apps", "", withCookie(admin))
	if !strings.Contains(all, appA) || !strings.Contains(all, appB) {
		t.Errorf("instance admin should see every team: %s", all)
	}
}

// TestTeamIsolation_SwitcherOnlyNarrows checks the scope cookie and header
// narrow a list and never widen it.
func TestTeamIsolation_SwitcherOnlyNarrows(t *testing.T) {
	f := newTeamFixture(t)
	owner := f.cookies[store.TeamRoleOwner]
	_, body := teamDo(t, f.rt, http.MethodGet, "/api/v1/apps", "", func(r *http.Request) {
		r.AddCookie(owner)
		r.Header.Set(scopeTeamHeader, teamB)
	})
	if strings.Contains(body, appB) || strings.Contains(body, appA) {
		t.Errorf("narrowing to a team the caller is not in must show nothing: %s", body)
	}
	_, body = teamDo(t, f.rt, http.MethodGet, "/api/v1/apps", "", func(r *http.Request) {
		r.AddCookie(owner)
		r.AddCookie(&http.Cookie{Name: scopeProjectCookie, Value: projA2}) //nolint:gosec // request cookie, not a response Set-Cookie
	})
	if !strings.Contains(body, appA2) || strings.Contains(body, `"`+appA+`"`) {
		t.Errorf("project narrowing: %s", body)
	}
}

func mintTeamToken(t *testing.T, f *teamFixture, role, body string) (int, string) {
	t.Helper()
	code, out := teamDo(t, f.rt, http.MethodPost, "/api/v1/auth/tokens", body, withCookie(f.cookies[role]))
	if code != http.StatusCreated {
		return code, out
	}
	var res createTokenResponse
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	return code, res.Token
}

// TestTeamTokens_ScopeIsEnforced covers team and project limited tokens.
func TestTeamTokens_ScopeIsEnforced(t *testing.T) {
	f := newTeamFixture(t)
	tests := []struct {
		name, body string
		wantCode   int
	}{
		{"role above owner's own", `{"name":"x","abilities":["read"],"team_id":"` + teamA + `","team_role":"owner"}`, http.StatusBadRequest},
		{"team the user is not in", `{"name":"x","abilities":["read"],"team_id":"` + teamB + `"}`, http.StatusBadRequest},
		{"project of another team", `{"name":"x","abilities":["read"],"team_id":"` + teamA + `","project_id":"` + projB + `"}`, http.StatusBadRequest},
		{"ability beyond the token role", `{"name":"x","abilities":["deploy"],"team_id":"` + teamA + `","team_role":"viewer"}`, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if code, out := mintTeamToken(t, f, store.TeamRoleDeveloper, tc.body); code != tc.wantCode {
				t.Errorf("mint = %d %s, want %d", code, out, tc.wantCode)
			}
		})
	}

	code, viewerTok := mintTeamToken(t, f, store.TeamRoleOwner, `{"name":"v","abilities":["read"],"team_id":"`+teamA+`","team_role":"viewer"}`)
	if code != http.StatusCreated {
		t.Fatalf("mint viewer token = %d %s", code, viewerTok)
	}
	if got, _ := teamDo(t, f.rt, http.MethodGet, "/api/v1/apps/"+appA, "", bearer(viewerTok)); got != http.StatusOK {
		t.Errorf("viewer token read own app = %d", got)
	}
	if got, _ := teamDo(t, f.rt, http.MethodPost, "/api/v1/apps/"+appA+"/restart", "", bearer(viewerTok)); got != http.StatusForbidden {
		t.Errorf("viewer token restart = %d, want 403", got)
	}
	if got, _ := teamDo(t, f.rt, http.MethodGet, "/api/v1/apps/"+appB, "", bearer(viewerTok)); got != http.StatusNotFound {
		t.Errorf("viewer token other team = %d, want 404", got)
	}

	code, projTok := mintTeamToken(t, f, store.TeamRoleOwner, `{"name":"p","abilities":["read"],"team_id":"`+teamA+`","project_id":"`+projA2+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("mint project token = %d %s", code, projTok)
	}
	if got, _ := teamDo(t, f.rt, http.MethodGet, "/api/v1/apps/"+appA2, "", bearer(projTok)); got != http.StatusOK {
		t.Errorf("project token own project app = %d", got)
	}
	if got, _ := teamDo(t, f.rt, http.MethodGet, "/api/v1/apps/"+appA, "", bearer(projTok)); got != http.StatusNotFound {
		t.Errorf("project token sibling project app = %d, want 404", got)
	}
	if got, _ := teamDo(t, f.rt, http.MethodGet, "/api/v1/nodes", "", bearer(projTok)); got != http.StatusForbidden {
		t.Errorf("project token instance route = %d, want 403", got)
	}

	rootTok := seedMatrixToken(t, f.db, "root-scoped", []string{AbilityRoot})
	if err := f.db.SetAPITokenScope(context.Background(), store.APITokenScope{TokenID: "root-scoped", TeamID: teamB, Role: store.TeamRoleViewer}); err != nil {
		t.Fatal(err)
	}
	if got, _ := teamDo(t, f.rt, http.MethodGet, "/api/v1/apps/"+appA, "", bearer(rootTok)); got != http.StatusNotFound {
		t.Errorf("system token scoped to team B reads team A = %d, want 404", got)
	}
	if got, _ := teamDo(t, f.rt, http.MethodDelete, "/api/v1/apps/"+appB, "", bearer(rootTok)); got != http.StatusForbidden {
		t.Errorf("viewer scoped system token deletes = %d, want 403", got)
	}
}

// TestTeamAccess_CannotFileIntoAnotherTeam: creating or moving an app into
// another team's project, or leaving it project-less as a tenant, is refused.
func TestTeamAccess_CannotFileIntoAnotherTeam(t *testing.T) {
	f := newTeamFixture(t)
	dev := withCookie(f.cookies[store.TeamRoleDeveloper])
	cases := []struct {
		method, path, body string
	}{
		{http.MethodPost, "/api/v1/apps", `{"name":"sneak","image":"nginx:1","port":80,"project_id":"` + projB + `"}`},
		{http.MethodPost, "/api/v1/apps", `{"name":"orphan","image":"nginx:1","port":80}`},
		{http.MethodPut, "/api/v1/apps/" + appA + "/project", `{"project_id":"` + projB + `"}`},
		{http.MethodPut, "/api/v1/apps/" + appA + "/project", `{"project_id":""}`},
		{http.MethodPut, "/api/v1/projects/" + projA + "/organization", `{"org_id":"` + teamB + `"}`},
		{http.MethodPost, "/api/v1/projects", `{"name":"p","team_id":"` + teamB + `"}`},
		{http.MethodDelete, "/api/v1/projects/" + projA, ""},
	}
	for _, tc := range cases {
		code, body := teamDo(t, f.rt, tc.method, tc.path, tc.body, dev)
		if code < 400 {
			t.Errorf("%s %s %s = %d %s, want refused", tc.method, tc.path, tc.body, code, body)
		}
	}
	code, body := teamDo(t, f.rt, http.MethodPost, "/api/v1/apps", `{"name":"mine","image":"nginx:1","port":80,"project_id":"`+projA+`"}`, dev)
	if code != http.StatusCreated {
		t.Fatalf("create in own project = %d %s", code, body)
	}
	if owner, _ := f.db.OwnerOfApp(context.Background(), "mine"); owner == nil || owner.TeamID != teamA {
		t.Errorf("new app owner = %+v, want team A", owner)
	}
}

// TestTeamAccess_ProtectedEnvironmentNeedsAdmin: a developer cannot change an
// app in a protected environment; an admin can.
func TestTeamAccess_ProtectedEnvironmentNeedsAdmin(t *testing.T) {
	f := newTeamFixture(t)
	protected := true
	if err := f.db.UpdateEnvironment(context.Background(), envA, store.EnvironmentPatch{Protected: &protected}); err != nil {
		t.Fatal(err)
	}
	if code, _ := teamDo(t, f.rt, http.MethodPost, "/api/v1/apps/"+appA+"/restart", "", withCookie(f.cookies[store.TeamRoleDeveloper])); code != http.StatusForbidden {
		t.Errorf("developer restart in protected env = %d, want 403", code)
	}
	if code, _ := teamDo(t, f.rt, http.MethodPost, "/api/v1/apps/"+appA+"/restart", "", withCookie(f.cookies[store.TeamRoleAdmin])); code == http.StatusForbidden || code == http.StatusNotFound {
		t.Errorf("admin restart in protected env = %d, want allowed past the gate", code)
	}
	approve := "/api/v1/deploy-approvals/" + approvalA + "/reject"
	if code, _ := teamDo(t, f.rt, http.MethodPost, approve, `{"reason":"no"}`, withCookie(f.cookies[store.TeamRoleDeveloper])); code != http.StatusForbidden {
		t.Errorf("developer decides approval = %d, want 403", code)
	}
	if code, body := teamDo(t, f.rt, http.MethodPost, approve, `{"reason":"no"}`, withCookie(f.cookies[store.TeamRoleAdmin])); code == http.StatusForbidden || code == http.StatusNotFound {
		t.Errorf("admin decides approval = %d %s, want allowed past the gate", code, body)
	}
}
