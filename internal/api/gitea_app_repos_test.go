package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/giteaapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// newTestRouterWithGiteaAndGitSource wires both a Gitea connection and
// git-source secrets into one router: handleUseGiteaRepoAsSource needs
// both, the same shape newTestRouterWithGitLabAndGitSource establishes.
func newTestRouterWithGiteaAndGitSource(t *testing.T, giteaSecrets GiteaAppSecrets, giteaClient GiteaAppClient, gitSourceSecrets GitSourceSecrets) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db,
		WithGiteaAppSecrets(giteaSecrets),
		WithGitSourceSecrets(gitSourceSecrets),
	)
	rt.giteaAppClient = giteaClient
	return rt, db
}

func authorizedGiteaSecrets(t *testing.T) *fakeGiteaAppSecrets {
	t.Helper()
	s := newFakeGiteaAppSecrets()
	key := store.GiteaAppSecretsKey()
	mustSetGiteaSecret(t, s, key, giteaAppClientSecretKey, "csecret")
	mustSetGiteaSecret(t, s, key, giteaAppAccessTokenKey, "at-1")
	mustSetGiteaSecret(t, s, key, giteaAppTokenExpiresKey, "99999999999")
	return s
}

func TestHandleListGiteaAppRepos_NotAuthorized(t *testing.T) {
	rt, db := newTestRouterWithGiteaApp(t, newFakeGiteaAppSecrets(), &fakeGiteaAppClient{})
	cookie := loginTestSession(t, rt, db)
	seedGiteaAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitea-app/repos", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListGiteaAppRepos_Success(t *testing.T) {
	fakeClient := &fakeGiteaAppClient{
		repos: []giteaapp.Repo{
			{FullName: "acme/widgets", Name: "widgets", CloneURL: "https://git.example.com/acme/widgets.git", DefaultBranch: "main", Private: true},
		},
	}
	secrets := authorizedGiteaSecrets(t)
	rt, db := newTestRouterWithGiteaApp(t, secrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedGiteaAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitea-app/repos", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"full_name":"acme/widgets"`) {
		t.Errorf("body = %s, want the repo's full_name", rec.Body.String())
	}
}

func TestHandleListGiteaAppBranches_NotAuthorized(t *testing.T) {
	rt, db := newTestRouterWithGiteaApp(t, newFakeGiteaAppSecrets(), &fakeGiteaAppClient{})
	cookie := loginTestSession(t, rt, db)
	seedGiteaAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitea-app/repos/acme/widgets/branches", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListGiteaAppBranches_Success(t *testing.T) {
	fakeClient := &fakeGiteaAppClient{
		branches: []giteaapp.Branch{
			{Name: "main", CommitSHA: "abc123"},
			{Name: "dev", CommitSHA: "def456"},
		},
	}
	secrets := authorizedGiteaSecrets(t)
	rt, db := newTestRouterWithGiteaApp(t, secrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedGiteaAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitea-app/repos/acme/widgets/branches", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"name":"main"`) || !strings.Contains(body, `"name":"dev"`) {
		t.Errorf("body = %s, missing expected branches", body)
	}
}

func TestHandleListGiteaAppBranches_UpstreamListError(t *testing.T) {
	fakeClient := &fakeGiteaAppClient{branchesErr: errFakeSecretNotSet}
	secrets := authorizedGiteaSecrets(t)
	rt, db := newTestRouterWithGiteaApp(t, secrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedGiteaAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitea-app/repos/acme/widgets/branches", ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body = %s", rec.Code, rec.Body.String())
	}
}

func TestGiteaAppRepoRoutes_PlainReadTokenForbidden(t *testing.T) {
	rt, db := newTestRouterWithGiteaApp(t, newFakeGiteaAppSecrets(), &fakeGiteaAppClient{})

	const plaintext = "read-only-token-gitea" //nolint:gosec // fake fixture, not a real credential
	assertProviderRoutesForbiddenForAbilities(t, rt, db, "tok_read_gitea", plaintext, []string{AbilityRead}, []providerRouteCase{
		{method: http.MethodGet, path: "/api/v1/gitea-app/repos"},
	})
}

func TestHandleUseGiteaRepoAsSource_AppNotFound(t *testing.T) {
	secrets := authorizedGiteaSecrets(t)
	rt, db := newTestRouterWithGiteaAndGitSource(t, secrets, &fakeGiteaAppClient{}, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedGiteaAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/gitea-app/repos/acme/missing/use-as-source", `{"app_name":"missing"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

// TestHandleUseGiteaRepoAsSource_Success proves this handler ends up
// creating the exact same store.GitSource row (and webhook secret under
// the exact same GitSourceSecretsKey) that PUT .../git-source itself
// creates, and registers a Gitea repo webhook pointed at that source's
// own webhook URL with that same secret.
func TestHandleUseGiteaRepoAsSource_Success(t *testing.T) {
	fakeClient := &fakeGiteaAppClient{
		repo: giteaapp.Repo{
			FullName: "acme/web", Name: "web",
			CloneURL: "https://git.example.com/acme/web.git", DefaultBranch: "trunk",
		},
	}
	secrets := authorizedGiteaSecrets(t)
	gitSourceSecrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGiteaAndGitSource(t, secrets, fakeClient, gitSourceSecrets)
	cookie := loginTestSession(t, rt, db)
	seedGiteaAppConnection(t, db)
	setPrimaryDomain(t, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/gitea-app/repos/acme/web/use-as-source", `{"app_name":"web"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"repo_url":"https://git.example.com/acme/web.git"`) {
		t.Errorf("body = %s, want repo_url from the looked-up repo", body)
	}
	if !strings.Contains(body, `"branch":"trunk"`) {
		t.Errorf("body = %s, want branch defaulted from the repo's own default_branch", body)
	}
	if !strings.Contains(body, `"webhook_secret"`) {
		t.Errorf("body = %s, want webhook_secret on first connect", body)
	}

	saved, err := db.GetGitSource(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetGitSource() error = %v", err)
	}
	if saved.RepoURL != "https://git.example.com/acme/web.git" {
		t.Errorf("saved git source repo_url = %q, want the gitea repo's clone url", saved.RepoURL)
	}

	assertGitSourceWebhookRegistered(t, gitSourceSecrets, "web", fakeClient.createHookCall, fakeClient.createHookURL, fakeClient.createHookTok)
}

func TestHandleUseGiteaRepoAsSource_WebhookRegistrationFails(t *testing.T) {
	fakeClient := &fakeGiteaAppClient{
		repo:          giteaapp.Repo{FullName: "acme/web", CloneURL: "https://git.example.com/acme/web.git", DefaultBranch: "main"},
		createHookErr: errFakeSecretNotSet,
	}
	secrets := authorizedGiteaSecrets(t)
	rt, db := newTestRouterWithGiteaAndGitSource(t, secrets, fakeClient, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedGiteaAppConnection(t, db)
	setPrimaryDomain(t, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/gitea-app/repos/acme/web/use-as-source", `{"app_name":"web"}`))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body = %s", rec.Code, rec.Body.String())
	}

	assertGitSourceSurvivesWebhookFailure(t, db)
}
