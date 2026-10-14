package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type bodyRefCase struct {
	name               string
	method, path, body string
	wantStatus         int
	wantBody           string
}

// bodyRefFixture is a root user with explicit IAM Denies on one resource of
// each kind; the open twin of each stays reachable.
func bodyRefFixture(t *testing.T, rt *Router, db *store.DB) *http.Cookie {
	t.Helper()
	bootstrapTestAdmin(t, db)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.SaveProject(ctx, store.Project{ID: "proj-1", Name: "p1", CreatedAt: "2026-10-01T00:00:00Z"}))
	for _, env := range []string{"env-ok", "env-deny"} {
		must(db.SaveEnvironment(ctx, store.Environment{ID: env, ProjectID: "proj-1", Name: env, CreatedAt: "2026-10-01T00:00:00Z"}))
	}
	for _, app := range []string{"web", "victim", "open", "x-api"} {
		must(db.SaveDesiredService(ctx, store.DesiredService{Name: app, Image: "nginx:1", Port: 80}))
		must(db.UpdateServiceProject(ctx, app, "proj-1"))
	}
	must(db.SetServiceEnvironment(ctx, "victim", "env-ok"))
	must(db.SetServiceEnvironment(ctx, "open", "env-ok"))
	for _, d := range []string{"main", "secret-db"} {
		must(db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: d, Engine: store.EnginePostgres, Version: "16"}))
	}
	for _, n := range []string{"n-ok", "n-deny"} {
		must(db.SaveNode(ctx, store.Node{ID: n, Name: n, Status: store.NodeStatusOnline, Schedulable: true}))
	}
	user := storeUserWithAbilitiesForTest(t, db, "operator@example.com", []string{AbilityRoot})
	for _, res := range []string{"app:victim", "app:x-api", "database:secret-db", "node:n-deny", "environment:env-deny"} {
		attachTestPolicy(t, db, "deny-"+res, "Deny", "*", res, store.PrincipalTypeUser, user.ID)
	}
	return sessionCookieForTest(t, rt, user.ID)
}

func runBodyRefCases(t *testing.T, rt *Router, cookie *http.Cookie, cases []bodyRefCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, c.method, c.path, c.body))
			if rec.Code != c.wantStatus || !strings.Contains(rec.Body.String(), c.wantBody) {
				t.Fatalf("%s %s = %d %s, want %d containing %q", c.method, c.path, rec.Code, rec.Body.String(), c.wantStatus, c.wantBody)
			}
		})
	}
}

func TestBodyRefs_IAMDenyOnReferencedResource(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	cookie := bodyRefFixture(t, rt, db)
	denyPipeline := jsonBody(t, map[string]string{"name": "p", "yaml": "version: 1\njobs:\n  p:\n    steps:\n      - uses: deploy\n        with: { service: victim, image: x }\n"})
	openPipeline := jsonBody(t, map[string]string{"name": "q", "yaml": "version: 1\njobs:\n  p:\n    steps:\n      - uses: deploy\n        with: { service: open, image: x }\n"})
	runBodyRefCases(t, rt, cookie, []bodyRefCase{
		{"app node denied", http.MethodPut, "/api/v1/apps/web/node", `{"node_id":"n-deny"}`, http.StatusBadRequest, "unknown node_id"},
		{"app node unknown answers the same", http.MethodPut, "/api/v1/apps/web/node", `{"node_id":"n-nope"}`, http.StatusBadRequest, "unknown node_id"},
		{"app node allowed", http.MethodPut, "/api/v1/apps/web/node", `{"node_id":"n-ok"}`, http.StatusOK, `"node_id":"n-ok"`},
		{"database node denied", http.MethodPut, "/api/v1/databases/main/node", `{"node_id":"n-deny"}`, http.StatusBadRequest, "unknown node_id"},
		{"database node allowed", http.MethodPut, "/api/v1/databases/main/node", `{"node_id":"n-ok"}`, http.StatusOK, "n-ok"},
		{"create app on denied node", http.MethodPost, "/api/v1/apps", `{"name":"fresh","image":"nginx:1","port":80,"node_id":"n-deny"}`, http.StatusBadRequest, "unknown node_id"},
		{"app environment denied", http.MethodPut, "/api/v1/apps/web/environment", `{"environment_id":"env-deny"}`, http.StatusBadRequest, "unknown environment_id"},
		{"app environment allowed", http.MethodPut, "/api/v1/apps/web/environment", `{"environment_id":"env-ok"}`, http.StatusOK, "env-ok"},
		{"database environment denied", http.MethodPut, "/api/v1/databases/main/environment", `{"environment_id":"env-deny"}`, http.StatusBadRequest, "unknown environment_id"},
		{"clone into denied environment", http.MethodPost, "/api/v1/apps/open/clone", `{"new_name":"open2","environment_id":"env-deny"}`, http.StatusBadRequest, errCloneEnvironmentMsg},
		{"clone into allowed environment", http.MethodPost, "/api/v1/apps/open/clone", `{"new_name":"open3","environment_id":"env-ok"}`, http.StatusCreated, "open3"},
		{"promote to denied environment", http.MethodPost, "/api/v1/apps/web/promote", `{"to":"env-deny"}`, http.StatusBadRequest, "unknown environment: env-deny"},
		{"promote into denied target", http.MethodPost, "/api/v1/apps/web/promote", `{"to":"env-ok","target":"victim"}`, http.StatusNotFound, "target app not found: victim"},
		{"compose overwrites denied app", http.MethodPost, "/api/v1/apps/x/compose", "services:\n  api:\n    image: nginx:1\n", http.StatusConflict, "an app named x-api already exists"},
		{"compose new service allowed", http.MethodPost, "/api/v1/apps/y/compose", "services:\n  api:\n    image: nginx:1\n", http.StatusOK, "y-api"},
		{"reverse proxy guide denied app", http.MethodGet, "/api/v1/system/reverse-proxy?app=victim", "", http.StatusNotFound, "app victim not found"},
		{"pipeline deploying a denied app", http.MethodPost, "/api/v1/apps/web/pipelines", denyPipeline, http.StatusBadRequest, "app not found"},
		{"pipeline deploying an open app", http.MethodPost, "/api/v1/apps/web/pipelines", openPipeline, http.StatusCreated, `"name":"q"`},
		{"standalone domain to denied app", http.MethodPost, "/api/v1/domains/standalone", `{"domain":"x.example.com","target":"app:victim"}`, http.StatusBadRequest, `app \"victim\" does not exist`},
		{"external database on denied node", http.MethodPost, "/api/v1/external-databases", `{"name":"ext","engine":"postgres","host":"db.example.com","port":5432,"node_id":"n-deny"}`, http.StatusBadGateway, "not currently reachable"},
		{"external database in unknown project", http.MethodPost, "/api/v1/external-databases", `{"name":"ext","engine":"postgres","host":"db.example.com","port":5432,"project_id":"proj-nope"}`, http.StatusBadRequest, "unknown project_id"},
	})
}

func TestBodyRefs_EnvironmentCloneSkipsDeniedApps(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := bodyRefFixture(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/environments/env-ok/clone", `{"new_environment_name":"staging"}`))
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("clone = %d %s", rec.Code, rec.Body.String())
	}
	var got environmentCloneResultResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var sources []string
	for _, a := range got.Apps {
		sources = append(sources, a.SourceApp)
	}
	if len(sources) != 1 || sources[0] != "open" {
		t.Fatalf("cloned sources = %v, want only open (victim is denied)", sources)
	}
}

func TestBodyRefs_GitSourceAdditionalServiceDenied(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := bodyRefFixture(t, rt, db)
	runBodyRefCases(t, rt, cookie, []bodyRefCase{
		{"denied", http.MethodPut, "/api/v1/apps/web/git-source", `{"repo_url":"https://github.com/org/web.git","additional_services":{"victim":{"build_type":"dockerfile"}}}`, http.StatusBadRequest, "app not found"},
		{"allowed", http.MethodPut, "/api/v1/apps/web/git-source", `{"repo_url":"https://github.com/org/web.git","additional_services":{"open":{"build_type":"dockerfile"}}}`, http.StatusCreated, `"open"`},
	})
}

func TestBodyRefs_DeploySpecSingleServiceNameDenied(t *testing.T) {
	rt, db := newTestRouterWithBuilderAndSecrets(t, &fakeBuilder{tag: "img:sha"}, newFakeFetch("/tmp/checkout", nil), &fakeSecretSetter{})
	cookie := bodyRefFixture(t, rt, db)
	body := `{"repo_url":"https://example.com/r.git","ref":"main","single_service_name":"victim","services":{"web":{"build":{"type":"dockerfile","path":"./Dockerfile"},"port":3000}}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploy-spec", body))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "an app named victim already exists") {
		t.Fatalf("deploy-spec onto denied app = %d %s, want 409", rec.Code, rec.Body.String())
	}
}

func TestBodyRefs_AlertBackupDatabaseDenied(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := bodyRefFixture(t, rt, db)
	rule := func(dbName string) string {
		return `{"name":"bk","kind":"backup_missing","backup_resource_kind":"database","backup_database_name":"` + dbName + `","for_duration":"6h","notify_url":"https://example.com/hook","enabled":true}`
	}
	runBodyRefCases(t, rt, cookie, []bodyRefCase{
		{"denied", http.MethodPost, "/api/v1/apps/web/alerts", rule("secret-db"), http.StatusBadRequest, "unknown backup_database_name"},
		{"allowed", http.MethodPost, "/api/v1/apps/web/alerts", rule("main"), http.StatusCreated, "main"},
	})
}

// A tenant's reference to another team's environment answers exactly like an
// unknown id, so the response is not an existence oracle.
func TestBodyRefs_TenantCannotReferenceOtherTeamEnvironment(t *testing.T) {
	f := newTeamFixture(t)
	const envA2 = "env_team_a2"
	if err := f.db.SaveEnvironment(context.Background(), store.Environment{ID: envA2, ProjectID: projA, Name: "staging", CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	dev := withCookie(f.cookies[store.TeamRoleDeveloper])
	cases := []struct {
		name, method, path, body string
		wantStatus               int
		wantBody                 string
	}{
		{"move app to other team env", http.MethodPut, "/api/v1/apps/" + appA + "/environment", `{"environment_id":"` + envB + `"}`, http.StatusBadRequest, "unknown environment_id"},
		{"move app to unknown env", http.MethodPut, "/api/v1/apps/" + appA + "/environment", `{"environment_id":"env_nope"}`, http.StatusBadRequest, "unknown environment_id"},
		{"move database to other team env", http.MethodPut, "/api/v1/databases/" + dbA + "/environment", `{"environment_id":"` + envB + `"}`, http.StatusBadRequest, "unknown environment_id"},
		{"promote to other team env", http.MethodPost, "/api/v1/apps/" + appA + "/promote", `{"to":"` + envB + `"}`, http.StatusBadRequest, "unknown environment: " + envB},
		{"clone into other team env", http.MethodPost, "/api/v1/apps/" + appA + "/clone", `{"new_name":"a-copy","environment_id":"` + envB + `"}`, http.StatusBadRequest, errCloneEnvironmentMsg},
		{"move app within own team", http.MethodPut, "/api/v1/apps/" + appA + "/environment", `{"environment_id":"` + envA2 + `"}`, http.StatusOK, envA2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, body := teamDo(t, f.rt, c.method, c.path, c.body, dev)
			if code != c.wantStatus || !strings.Contains(body, c.wantBody) {
				t.Fatalf("%s %s = %d %s, want %d containing %q", c.method, c.path, code, body, c.wantStatus, c.wantBody)
			}
		})
	}
}
