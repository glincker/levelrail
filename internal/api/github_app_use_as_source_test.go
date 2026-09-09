package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// newTestRouterWithGitHubAndGitSource wires both a GitHub App connection
// and git-source secrets into one router: handleUseGitHubRepoAsSource
// needs both, the same shape newTestRouterWithGitLabAndGitSource already
// establishes for GitLab's own use-as-source handler.
func newTestRouterWithGitHubAndGitSource(t *testing.T, githubSecrets GitHubAppSecrets, githubClient GitHubAppClient, gitSourceSecrets GitSourceSecrets) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db,
		WithGitHubAppSecrets(githubSecrets),
		WithGitSourceSecrets(gitSourceSecrets),
	)
	rt.githubAppClient = githubClient
	return rt, db
}

func TestHandleUseGitHubRepoAsSource_AppNotFound(t *testing.T) {
	fakeSecrets := newFakeGitHubAppSecrets()
	rt, db := newTestRouterWithGitHubAndGitSource(t, fakeSecrets, &fakeGitHubAppClient{}, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, fakeSecrets)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/github-app/repos/acme/widgets/use-as-source", `{"app_name":"missing"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUseGitHubRepoAsSource_NotInstalled(t *testing.T) {
	rt, db := newTestRouterWithGitHubAndGitSource(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{}, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/github-app/repos/acme/widgets/use-as-source", `{"app_name":"web"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

// seedGitHubUseAsSourceFixture wires an installed GitHub App connection,
// a primary domain, and the target app "web": the common setup every
// Success/PermissionDenied/OtherWebhookFailure test below needs before
// hitting the handler, varying only the looked-up repo's default_branch
// and the fake client's own webhook-registration outcome.
func seedGitHubUseAsSourceFixture(t *testing.T, defaultBranch string, createHookErr error) (rt *Router, db *store.DB, fakeClient *fakeGitHubAppClient, gitSourceSecrets *fakeGitSourceSecrets, cookie *http.Cookie) {
	t.Helper()
	fakeSecrets := newFakeGitHubAppSecrets()
	fakeClient = &fakeGitHubAppClient{
		repo:          githubapp.Repo{FullName: "acme/web", DefaultBranch: defaultBranch},
		createHookErr: createHookErr,
	}
	gitSourceSecrets = newFakeGitSourceSecrets()
	rt, db = newTestRouterWithGitHubAndGitSource(t, fakeSecrets, fakeClient, gitSourceSecrets)
	cookie = loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, fakeSecrets)
	setPrimaryDomain(t, db)
	seedApp(t, db, "web")
	return rt, db, fakeClient, gitSourceSecrets, cookie
}

// TestHandleUseGitHubRepoAsSource_Success proves this handler connects
// the exact same store.GitSource row PUT .../git-source itself creates,
// and registers a GitHub repo webhook pointed at that source's own
// webhook URL with that same secret, reporting webhook_registered:true.
func TestHandleUseGitHubRepoAsSource_Success(t *testing.T) {
	rt, db, fakeClient, gitSourceSecrets, cookie := seedGitHubUseAsSourceFixture(t, "trunk", nil)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/github-app/repos/acme/web/use-as-source", `{"app_name":"web"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"repo_url":"https://github.com/acme/web.git"`) {
		t.Errorf("body = %s, want repo_url derived from the looked-up repo", body)
	}
	if !strings.Contains(body, `"branch":"trunk"`) {
		t.Errorf("body = %s, want branch defaulted from the repo's own default_branch", body)
	}
	if !strings.Contains(body, `"webhook_registered":true`) {
		t.Errorf("body = %s, want webhook_registered:true", body)
	}
	if strings.Contains(body, `"webhook_error"`) {
		t.Errorf("body = %s, want no webhook_error on success", body)
	}

	saved, err := db.GetGitSource(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetGitSource() error = %v", err)
	}
	if saved.RepoURL != "https://github.com/acme/web.git" {
		t.Errorf("saved git source repo_url = %q, want the github repo's clone url", saved.RepoURL)
	}

	assertGitSourceWebhookRegistered(t, gitSourceSecrets, "web", fakeClient.createHookCall, fakeClient.createHookURL, fakeClient.createHookSec)
}

// TestHandleUseGitHubRepoAsSource_PermissionDenied is the test proving
// the actual degrade path this task cared most about: an installation
// whose CreateRepoWebhook call fails with githubapp.ErrPermissionDenied
// (the "installed before repository_hooks:write was requested" case)
// still ends up with a connected git source, a 2xx response, and a
// webhook_error explaining the manual fallback, not a failed request.
func TestHandleUseGitHubRepoAsSource_PermissionDenied(t *testing.T) {
	rt, db, _, _, cookie := seedGitHubUseAsSourceFixture(t, "main", githubapp.ErrPermissionDenied)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/github-app/repos/acme/web/use-as-source", `{"app_name":"web"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (git source still connects despite the permission error), body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"webhook_registered":false`) {
		t.Errorf("body = %s, want webhook_registered:false", body)
	}
	if !strings.Contains(body, `"webhook_error"`) {
		t.Errorf("body = %s, want a webhook_error explaining the manual fallback", body)
	}
	if !strings.Contains(body, `"webhook_secret"`) {
		t.Errorf("body = %s, want webhook_secret present so the frontend can render the manual-add banner", body)
	}

	// The git source is connected even though webhook registration was
	// denied: the whole point of degrading here instead of failing is
	// that the operator ends up with a working (if not yet auto-deploying)
	// connection, not stuck unable to connect GitHub at all.
	assertGitSourceSurvivesWebhookFailure(t, db)
}

// TestHandleUseGitHubRepoAsSource_OtherWebhookFailure proves a
// non-permission webhook registration failure still fails the request
// (502), the same "connect first, webhook second, but a bad failure is
// still a failure" shape GitLab/Bitbucket's own use-as-source handlers
// have: only the specific, structural permission-denied case degrades.
func TestHandleUseGitHubRepoAsSource_OtherWebhookFailure(t *testing.T) {
	rt, db, _, _, cookie := seedGitHubUseAsSourceFixture(t, "main", errFakeSecretNotSet)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/github-app/repos/acme/web/use-as-source", `{"app_name":"web"}`))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body = %s", rec.Code, rec.Body.String())
	}

	assertGitSourceSurvivesWebhookFailure(t, db)
}

func TestGitHubAppUseAsSourceRoute_PlainWriteTokenForbidden(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})

	const plaintext = "write-only-token-github" //nolint:gosec // fake fixture, not a real credential
	assertRoutesForbiddenForAbilities(t, rt, db, "tok_write_gh", plaintext, []string{AbilityWrite}, []routeCase{
		{method: http.MethodPost, path: "/api/v1/github-app/repos/acme/web/use-as-source", body: `{"app_name":"web"}`},
	})
}
