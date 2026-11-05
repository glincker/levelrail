package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCITemplatesFillInTheApp(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/ci/templates", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	var res ciTemplatesResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, tmpl := range res.Templates {
		ids[tmpl.ID] = tmpl.Auth
		if strings.Contains(tmpl.Content, "my-app") || strings.Contains(tmpl.Content, "example/web") {
			t.Errorf("%s: app name not filled in, or an owner was personalised", tmpl.ID)
		}
		if !strings.Contains(tmpl.Content, "apps deploy web") && !strings.Contains(tmpl.Content, "APP: web") {
			t.Errorf("%s: does not deploy app web", tmpl.ID)
		}
	}
	for id, auth := range map[string]string{"github": "oidc", "gitlab": "oidc", "circleci": "oidc", "jenkins": "token", "buildkite": "token"} {
		if ids[id] != auth {
			t.Errorf("provider %s auth = %q, want %q", id, ids[id], auth)
		}
	}
	if strings.Join(res.DeployerAbilities, ",") != "read,deploy" || !res.NeedsDomain {
		t.Errorf("resource = abilities %v needsDomain %v", res.DeployerAbilities, res.NeedsDomain)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/ci/templates", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown app = %d, want 404", rec.Code)
	}
}

func TestCITemplatesDeployerScopeFollowsTheAppsProject(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	get := func() ciTemplatesResource {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/ci/templates", ""))
		var res ciTemplatesResource
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		return res
	}
	if get().DeployerScope != nil {
		t.Fatal("an app in no project must not get a token scope")
	}
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "p", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateServiceProject(ctx, "web", "proj_1"); err != nil {
		t.Fatal(err)
	}
	scope := get().DeployerScope
	if scope == nil || scope.ProjectID != "proj_1" || scope.TeamID == "" || scope.TeamRole != store.TeamRoleDeveloper {
		t.Fatalf("scope = %+v", scope)
	}
}

func TestCITemplatesNeedTheReadAbility(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	seedApp(t, db, "web")
	if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: "tok_none", Name: "none", TokenHash: hashToken("none-secret"), Abilities: []string{AbilityDeploy}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/ci/templates", nil)
	req.Header.Set("Authorization", "Bearer none-secret")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a token without read = %d, want 403", rec.Code)
	}
}

func TestCITrustRefusesTheOwnerRepoPlaceholder(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	for _, body := range []string{
		`{"repository":"OWNER/REPO","ref_pattern":"refs/heads/main"}`,
		`{"provider":"gitlab","repository":"owner/repo","ref_pattern":"refs/heads/main"}`,
		`{"repository":"owner/name","ref_pattern":"refs/heads/main"}`,
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/ci/trusts", body))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "placeholder") {
			t.Fatalf("%s = %d %s, want 400 naming the placeholder", body, rec.Code, rec.Body)
		}
	}
}

// failingTeamOfProject makes the project to team lookup fail the way a transient database error would.
type failingTeamOfProject struct{ *store.DB }

func (failingTeamOfProject) TeamOfProject(context.Context, string) (string, error) {
	return "", errors.New("injected team lookup failure")
}

func TestCITemplatesFailInsteadOfFallingBackToAnInstallWideToken(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "p", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateServiceProject(ctx, "web", "proj_1"); err != nil {
		t.Fatal(err)
	}
	rt.policies = failingTeamOfProject{db}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/ci/templates", ""))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "deployer_abilities") {
		t.Fatalf("status = %d body = %s, want a 500 with no templates to build an unscoped token from", rec.Code, rec.Body)
	}
}
