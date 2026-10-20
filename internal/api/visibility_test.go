package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type visibilityFixture struct {
	rt        *Router
	db        *store.DB
	guest     store.User
	operator  store.User
	guestTok  string
	ownerless string
}

func newVisibilityFixture(t *testing.T) visibilityFixture {
	t.Helper()
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	ctx := context.Background()
	now := time.Now().UTC()
	apps := map[string]string{"dev-web": "env_dev", "prod-web": "env_production", "loose-web": ""}
	for name, env := range apps {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: name + ":1", Port: 80}); err != nil {
			t.Fatal(err)
		}
		if env != "" {
			if err := db.SetServiceEnvironment(ctx, name, env); err != nil {
				t.Fatal(err)
			}
		}
		if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
			ID: "dep_" + name, ServiceName: name, Image: name + ":1", Source: store.DeployAttemptSourceImage,
			Status: store.DeployAttemptStatusFailed, StartedAt: now.Add(-time.Minute), Error: "boom",
		}); err != nil {
			t.Fatal(err)
		}
		if err := db.SaveDeployApproval(ctx, store.DeployApproval{
			ID: "apr_" + name, ServiceName: name, EnvironmentID: "env_production",
			Action: store.DeployApprovalActionDeploy, Image: name + ":2", Status: store.DeployApprovalStatusPending,
			RequestedByType: store.PrincipalTypeUser, RequestedBy: "u", RequestedByName: "U",
			CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Hour).Format(time.RFC3339Nano),
		}); err != nil {
			t.Fatal(err)
		}
	}
	for name, env := range map[string]string{"dev-db": "env_dev", "prod-db": "env_production", "loose-db": ""} {
		if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: name, Engine: store.EngineRedis, Version: "7"}); err != nil {
			t.Fatal(err)
		}
		if env != "" {
			if _, err := db.ExecContext(ctx, `UPDATE desired_databases SET environment_id = ? WHERE name = ?`, env, name); err != nil {
				t.Fatal(err)
			}
		}
	}

	guest := storeUserWithAbilitiesForTest(t, db, "guest@example.com", []string{AbilityRead, AbilityWrite, AbilityDeploy})
	if err := db.CreateRole(ctx, store.Role{ID: "role_env_writer", Name: "env-writer", Abilities: []string{AbilityRead, AbilityWrite, AbilityDeploy}, Visibility: store.RoleVisibilityGranted}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserRole(ctx, guest.ID, "role_env_writer"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserEnvironmentGrants(ctx, guest.ID, []string{"env_dev"}); err != nil {
		t.Fatal(err)
	}
	operator := storeUserWithAbilitiesForTest(t, db, "op@example.com", []string{AbilityRead, AbilityWrite, AbilityDeploy})

	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "sys-tok", Name: "sys-tok", TokenHash: hashToken("sys-secret"), Abilities: []string{AbilityRead, AbilityWrite},
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, sessionCookieForTest(t, rt, guest.ID), http.MethodPost, "/api/v1/auth/tokens", `{"name":"guest-tok","abilities":["read","write","deploy"]}`))
	var minted createTokenResponse
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &minted) != nil {
		t.Fatalf("mint guest token: %d %s", rec.Code, rec.Body.String())
	}
	return visibilityFixture{rt: rt, db: db, guest: guest, operator: operator, guestTok: minted.Token, ownerless: "sys-secret"}
}

type principalCase struct {
	name string
	as   func(t *testing.T, f visibilityFixture, r *http.Request)
}

func (f visibilityFixture) do(t *testing.T, as func(*testing.T, visibilityFixture, *http.Request), method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if body != "" {
		req = httptest.NewRequest(method, path, stringsReader(body))
	}
	as(t, f, req)
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, req)
	return rec
}

func asGuestSession(t *testing.T, f visibilityFixture, r *http.Request) {
	r.AddCookie(sessionCookieForTest(t, f.rt, f.guest.ID))
}

func asGuestToken(_ *testing.T, f visibilityFixture, r *http.Request) { bearer(f.guestTok)(r) }

func asOperator(t *testing.T, f visibilityFixture, r *http.Request) {
	r.AddCookie(sessionCookieForTest(t, f.rt, f.operator.ID))
}

func asSystemToken(_ *testing.T, f visibilityFixture, r *http.Request) { bearer(f.ownerless)(r) }

var restrictedPrincipals = []principalCase{{"guest session", asGuestSession}, {"token of guest owner", asGuestToken}}
var unrestrictedPrincipals = []principalCase{{"operator session", asOperator}, {"ownerless token", asSystemToken}}

func TestGuestVisibility_Lists(t *testing.T) {
	f := newVisibilityFixture(t)
	names := func(t *testing.T, rec *httptest.ResponseRecorder, extract func([]byte) []string) []string {
		t.Helper()
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		got := extract(rec.Body.Bytes())
		slices.Sort(got)
		return got
	}
	type nameRow struct {
		Name        string `json:"name"`
		ServiceName string `json:"service_name"`
	}
	flat := func(b []byte) []string {
		var rows []nameRow
		_ = json.Unmarshal(b, &rows)
		out := []string{}
		for _, r := range rows {
			out = append(out, r.Name)
		}
		return out
	}
	deployments := func(b []byte) []string {
		var resp struct {
			Items []struct {
				App string `json:"app"`
			} `json:"items"`
		}
		_ = json.Unmarshal(b, &resp)
		out := []string{}
		for _, i := range resp.Items {
			out = append(out, i.App)
		}
		return out
	}
	approvals := func(b []byte) []string {
		var resp deployApprovalListResponse
		_ = json.Unmarshal(b, &resp)
		out := []string{}
		for _, a := range resp.Approvals {
			out = append(out, a.ServiceName)
		}
		return out
	}
	lists := []struct {
		path    string
		extract func([]byte) []string
		guest   []string
		all     []string
	}{
		{"/api/v1/apps", flat, []string{"dev-web"}, []string{"dev-web", "loose-web", "prod-web"}},
		{"/api/v1/databases", flat, []string{"dev-db"}, []string{"dev-db", "loose-db", "prod-db"}},
		{"/api/v1/deployments", deployments, []string{"dev-web"}, []string{"dev-web", "loose-web", "prod-web"}},
		{"/api/v1/deploy-approvals", approvals, []string{"dev-web"}, []string{"dev-web", "loose-web", "prod-web"}},
	}
	for _, l := range lists {
		for _, p := range restrictedPrincipals {
			t.Run(l.path+" "+p.name, func(t *testing.T) {
				got := names(t, f.do(t, p.as, http.MethodGet, l.path, ""), l.extract)
				if !slices.Equal(got, l.guest) {
					t.Errorf("got %v, want %v", got, l.guest)
				}
			})
		}
		for _, p := range unrestrictedPrincipals {
			t.Run(l.path+" "+p.name, func(t *testing.T) {
				got := names(t, f.do(t, p.as, http.MethodGet, l.path, ""), l.extract)
				if !slices.Equal(got, l.all) {
					t.Errorf("got %v, want %v", got, l.all)
				}
			})
		}
	}
}

func TestGuestVisibility_GatedRoutes(t *testing.T) {
	f := newVisibilityFixture(t)
	routes := []struct{ method, tmpl string }{
		{http.MethodGet, "/api/v1/apps/%s"},
		{http.MethodPut, "/api/v1/apps/%s"},
		{http.MethodGet, "/api/v1/apps/%s/group"},
		{http.MethodGet, "/api/v1/apps/%s/loadbalancer"},
		{http.MethodGet, "/api/v1/apps/%s/pipelines"},
		{http.MethodGet, "/api/v1/apps/%s/preview"},
		{http.MethodGet, "/api/v1/apps/%s/supply-chain"},
		{http.MethodGet, "/api/v1/databases/%s"},
		{http.MethodGet, "/api/v1/databases/%s/status"},
		{http.MethodDelete, "/api/v1/apps/%s"},
		{http.MethodDelete, "/api/v1/databases/%s"},
	}
	for _, rt := range routes {
		hidden, granted := []string{"prod-web", "loose-web", "no-such-app"}, "dev-web"
		if strings.Contains(rt.tmpl, "/databases/") {
			hidden, granted = []string{"prod-db", "loose-db", "no-such-db"}, "dev-db"
		}
		for _, p := range restrictedPrincipals {
			for _, h := range hidden {
				t.Run(rt.method+" "+rt.tmpl+" hidden "+h+" "+p.name, func(t *testing.T) {
					rec := f.do(t, p.as, rt.method, fmt.Sprintf(rt.tmpl, h), `{}`)
					if rec.Code != http.StatusNotFound {
						t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
					}
				})
			}
			t.Run(rt.method+" "+rt.tmpl+" granted "+p.name, func(t *testing.T) {
				rec := f.do(t, p.as, rt.method, fmt.Sprintf(rt.tmpl, granted), `{}`)
				if rt.method == http.MethodGet {
					want := f.do(t, asOperator, rt.method, fmt.Sprintf(rt.tmpl, granted), "").Code
					if rec.Code != want {
						t.Fatalf("guest status %d differs from operator %d", rec.Code, want)
					}
				}
			})
		}
	}
}

func TestGuestVisibility_OperatorUnaffected(t *testing.T) {
	f := newVisibilityFixture(t)
	for _, p := range unrestrictedPrincipals {
		for _, path := range []string{"/api/v1/apps/prod-web", "/api/v1/apps/loose-web", "/api/v1/databases/prod-db", "/api/v1/databases/loose-db"} {
			t.Run(p.name+" "+path, func(t *testing.T) {
				if rec := f.do(t, p.as, http.MethodGet, path, ""); rec.Code != http.StatusOK {
					t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
				}
			})
		}
	}
}

func TestGuestVisibility_GrantedReadIsOK(t *testing.T) {
	f := newVisibilityFixture(t)
	for _, p := range restrictedPrincipals {
		for _, path := range []string{"/api/v1/apps/dev-web", "/api/v1/databases/dev-db"} {
			t.Run(p.name+" "+path, func(t *testing.T) {
				if rec := f.do(t, p.as, http.MethodGet, path, ""); rec.Code != http.StatusOK {
					t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
				}
			})
		}
	}
}

func TestGuestVisibility_BulkCannotTouchHiddenApps(t *testing.T) {
	f := newVisibilityFixture(t)
	rec := f.do(t, asGuestSession, http.MethodPost, "/api/v1/apps/bulk", `{"action":"restart","names":["prod-web","dev-web"],"dry_run":true}`)
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp bulkAppsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range resp.Results {
		got[r.Name] = r.Status
	}
	if got["prod-web"] != bulkStatusDenied || got["dev-web"] != bulkStatusWouldRun {
		t.Fatalf("results = %v", got)
	}
}

func TestEnvironmentDenyPolicy_GateAndLists(t *testing.T) {
	f := newVisibilityFixture(t)
	attachTestPolicy(t, f.db, "no-prod-deploy", "Deny", "deploy", "environment-kind:production", store.PrincipalTypeUser, f.operator.ID)
	attachTestPolicy(t, f.db, "no-prod-read", "Deny", "read", "environment:env_production", store.PrincipalTypeUser, f.operator.ID)

	if rec := f.do(t, asOperator, http.MethodGet, "/api/v1/apps/prod-web", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("read prod status = %d, want 403", rec.Code)
	}
	if rec := f.do(t, asOperator, http.MethodGet, "/api/v1/apps/dev-web", ""); rec.Code != http.StatusOK {
		t.Fatalf("read dev status = %d, want 200", rec.Code)
	}
	if rec := f.do(t, asOperator, http.MethodGet, "/api/v1/apps/loose-web", ""); rec.Code != http.StatusOK {
		t.Fatalf("read untagged status = %d, want 200", rec.Code)
	}
	if rec := f.do(t, asOperator, http.MethodGet, "/api/v1/databases/prod-db", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("read prod db status = %d, want 403", rec.Code)
	}
	rec := f.do(t, asOperator, http.MethodGet, "/api/v1/apps", "")
	var rows []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Name == "prod-web" {
			t.Fatalf("prod-web listed despite environment read deny: %+v", rows)
		}
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want dev-web and loose-web", rows)
	}
}

func stringsReader(s string) *strings.Reader { return strings.NewReader(s) }
