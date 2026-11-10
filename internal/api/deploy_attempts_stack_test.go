package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/store"
)

func seedStackAttempt(t *testing.T, db *store.DB, stack string) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveApp(ctx, store.App{ID: stack, Name: stack, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{stack + "-web", stack + "-db"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: m, AppID: stack, Image: "nginx:1", Port: 80}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_" + stack, ServiceName: stack, Image: stack + ":abc", Source: store.DeployAttemptSourceWebhook,
		Status: store.DeployAttemptStatusFailed, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDeployAttempts_ComposeStackListedForOwner(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedStackAttempt(t, db, "shop")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/shop/deploy-attempts", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "dep_shop") {
		t.Fatalf("stack attempts = %d %s, want 200 with dep_shop", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/nope/deploy-attempts", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown name = %d, want 404", rec.Code)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/deployments", ""))
	var out deploymentListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.Items) != 1 || out.Items[0].App != "shop" {
		t.Fatalf("deployments = %d %s, want the stack row", rec.Code, rec.Body.String())
	}
}

func TestDeployAttempts_ComposeStackHiddenFromOtherOrg(t *testing.T) {
	f := newOrgFixture(t)
	ctx := context.Background()
	seedStackAttempt(t, f.db, "shopb")
	for _, m := range []string{"shopb-web", "shopb-db"} {
		if err := f.db.UpdateServiceProject(ctx, m, projB); err != nil {
			t.Fatal(err)
		}
		if err := f.db.SetServiceEnvironment(ctx, m, envB); err != nil {
			t.Fatal(err)
		}
	}
	path := "/api/v1/apps/shopb/deploy-attempts"
	if code, body := teamDo(t, f.rt, http.MethodGet, path, "", withCookie(f.bOwner)); code != http.StatusOK || !strings.Contains(body, "dep_shopb") {
		t.Fatalf("org B owner = %d %s, want its own stack attempts", code, body)
	}
	ownerA := withCookie(f.cookies[store.TeamRoleOwner])
	c1, b1 := teamDo(t, f.rt, http.MethodGet, path, "", ownerA)
	c2, b2 := teamDo(t, f.rt, http.MethodGet, "/api/v1/apps/zz-missing-stack/deploy-attempts", "", ownerA)
	if c1 != c2 || strings.Contains(b1, "dep_shopb") || (c1 == http.StatusOK && b1 != b2) {
		t.Fatalf("org A sees %d %q, a missing name gives %d %q", c1, b1, c2, b2)
	}
	_, list := teamDo(t, f.rt, http.MethodGet, "/api/v1/deployments", "", ownerA)
	if strings.Contains(list, "dep_shopb") {
		t.Fatalf("org A deployments list leaks the stack row: %s", list)
	}
}

func TestDeployments_IAMFilteredCallerSeesOwnStackOnly(t *testing.T) {
	rt, db := newTestRouter(t)
	seedStackAttempt(t, db, "shop")
	seedStackAttempt(t, db, "secret")
	tok := seedMatrixToken(t, db, "stack-tok", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-secret", "Deny", "*", "app:secret", store.PrincipalTypeToken, "stack-tok")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments", nil)
	bearer(tok)(req)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	var out deploymentListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("status = %d err = %v body = %s", rec.Code, err, rec.Body.String())
	}
	if len(out.Items) != 1 || out.Items[0].App != "shop" {
		t.Fatalf("filtered caller sees %+v, want only the shop stack", out.Items)
	}
}

func TestDeployAttempts_StackHiddenByMemberOrEnvironmentDeny(t *testing.T) {
	cases := []struct{ name, resource string }{
		{"environment deny", "environment:env_prod"},
		{"member deny", "app:shop-db"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			ctx := context.Background()
			seedStackAttempt(t, db, "shop")
			if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "p", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
				t.Fatal(err)
			}
			if err := db.SaveEnvironment(ctx, store.Environment{ID: "env_prod", ProjectID: "proj_1", Name: "production", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
				t.Fatal(err)
			}
			for _, m := range []string{"shop-web", "shop-db"} {
				if err := db.SetServiceEnvironment(ctx, m, "env_prod"); err != nil {
					t.Fatal(err)
				}
			}
			tok := seedMatrixToken(t, db, "deny-tok", []string{AbilityRead})
			attachTestPolicy(t, db, "deny-pol", "Deny", "*", tc.resource, store.PrincipalTypeToken, "deny-tok")

			get := func(path string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(http.MethodGet, path, nil)
				bearer(tok)(req)
				rec := httptest.NewRecorder()
				rt.Handler().ServeHTTP(rec, req)
				return rec
			}
			missing := get("/api/v1/apps/zz-missing/deploy-attempts")
			rec := get("/api/v1/apps/shop/deploy-attempts")
			if rec.Code != http.StatusNotFound || missing.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"error":"app not found"`) {
				t.Fatalf("denied stack = %d %q, want the unknown-name 404", rec.Code, rec.Body.String())
			}
			if list := get("/api/v1/deployments"); strings.Contains(list.Body.String(), "dep_shop") {
				t.Fatalf("deployments include the denied stack: %s", list.Body.String())
			}
		})
	}
}

func TestCloneIntoOtherTeamStackNameConflicts(t *testing.T) {
	f := newOrgFixture(t)
	ctx := context.Background()
	seedStackAttempt(t, f.db, "shopb")
	for _, m := range []string{"shopb-web", "shopb-db"} {
		if err := f.db.UpdateServiceProject(ctx, m, projB); err != nil {
			t.Fatal(err)
		}
		if err := f.db.SetServiceEnvironment(ctx, m, envB); err != nil {
			t.Fatal(err)
		}
	}
	ownerA := withCookie(f.cookies[store.TeamRoleOwner])
	code, body := teamDo(t, f.rt, http.MethodPost, "/api/v1/apps/"+appA+"/clone", `{"new_name":"shopb"}`, ownerA)
	if code != http.StatusConflict {
		t.Fatalf("clone onto another team's stack name = %d %s, want 409", code, body)
	}
	_, list := teamDo(t, f.rt, http.MethodGet, "/api/v1/deployments", "", ownerA)
	if strings.Contains(list, "dep_shopb") {
		t.Fatalf("org A deployments include the stack rows: %s", list)
	}
}

func seedProdStack(t *testing.T, db *store.DB, stack string) {
	t.Helper()
	ctx := context.Background()
	seedStackAttempt(t, db, stack)
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "p", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveEnvironment(ctx, store.Environment{ID: "env_prod", ProjectID: "proj_1", Name: "production", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{stack + "-web", stack + "-db"} {
		if err := db.SetServiceEnvironment(ctx, m, "env_prod"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSearch_EnvironmentDenyHidesStackDeployments(t *testing.T) {
	rt, db := newTestRouter(t)
	seedProdStack(t, db, "shop")
	tok := seedMatrixToken(t, db, "env-deny", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-env", "Deny", "*", "environment:env_prod", store.PrincipalTypeToken, "env-deny")
	for _, ref := range doSearch(t, rt, tok, "kind:deployment shop").refs() {
		if strings.Contains(ref, "dep_shop") {
			t.Fatalf("search returned the denied stack deployment %s", ref)
		}
	}
	allowed := seedMatrixToken(t, db, "plain", []string{AbilityRead})
	found := false
	for _, ref := range doSearch(t, rt, allowed, "kind:deployment shop").refs() {
		found = found || strings.Contains(ref, "dep_shop")
	}
	if !found {
		t.Fatal("an unrestricted caller no longer finds the stack deployment")
	}
}

func TestDeploymentsStream_EnvironmentDenySkipsStackEvents(t *testing.T) {
	rt, db := newTestRouter(t)
	recorder := deploylog.NewRecorder(nil, discardLogger())
	rt.deployRecorder = recorder
	ctx := context.Background()
	seedProdStack(t, db, "shop")
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "web:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_web", ServiceName: "web", Image: "web:1", Source: store.DeployAttemptSourceImage,
		Status: store.DeployAttemptStatusFailed, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	tok := seedMatrixToken(t, db, "env-deny", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-env", "Deny", "*", "environment:env_prod", store.PrincipalTypeToken, "env-deny")

	srv := httptest.NewServer(rt.Handler())
	reqCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(func() {
		cancel()
		srv.Close()
	})
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, srv.URL+"/api/v1/deployments/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	bearer(tok)(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d, want 200", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)

	recorder.Start("dep_shop")
	recorder.Start("dep_web")
	if ev := readStreamEvent(t, sc); ev.Deployment.ID != "dep_web" {
		t.Fatalf("first event is %q, want the stack event skipped", ev.Deployment.ID)
	}
}

func TestOtherMembersReadable_ServiceThatTookStackName(t *testing.T) {
	rt, db := newTestRouter(t)
	seedStackAttempt(t, db, "shop")
	ctx := context.Background()
	if !rt.otherMembersReadable(ctx, "shop", func(_, _ string) bool { return true }) {
		t.Fatal("readable members must pass")
	}
	if rt.otherMembersReadable(ctx, "shop", func(_, m string) bool { return m != "shop-db" }) {
		t.Fatal("an unreadable sharing member must fail")
	}
}

func TestStackNameCollisions(t *testing.T) {
	rt, db := newTestRouter(t)
	seedStackAttempt(t, db, "shopb")
	seedStackAttempt(t, db, "proj-shopb")
	ctx := context.Background()

	_, conflicts, err := rt.resolveClonedAppNames(ctx, []store.DesiredService{{Name: "web"}}, "staging", map[string]environmentCloneAppInput{"web": {NewName: "shopb"}})
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("environment clone onto a stack name: conflicts = %v err = %v, want one conflict", conflicts, err)
	}
	if _, err := (&importApplier{rt: rt}).CreateApp(ctx, platformimport.AppPlan{Name: "shopb", Image: "nginx:1", Port: 80, Replicas: 1}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("import onto a stack name: err = %v, want already exists", err)
	}
	for _, names := range [][]string{{"shopb"}, {"proj-shopb"}} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/other/deploy-spec", nil)
		if rt.authorizeServiceNames(rec, req, "other", names) || rec.Code != http.StatusConflict {
			t.Fatalf("deploy names %v = %d, want 409", names, rec.Code)
		}
	}
}
