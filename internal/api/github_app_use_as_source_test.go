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

// TestHandleUseGitHubRepoAsSource_Success proves this handler connects
// the exact same store.GitSource row PUT .../git-source itself creates,
// and registers a GitHub repo webhook pointed at that source's own
// webhook URL with that same secret, reporting webhook_registered:true.
func TestHandleUseGitHubRepoAsSource_Success(t *testing.T) {
	fakeSecrets := newFakeGitHubAppSecrets()
	fakeClient := &fakeGitHubAppClient{
		repo: githubapp.Repo{FullName: "acme/web", DefaultBranch: "trunk"},
	}
	gitSourceSecrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitHubAndGitSource(t, fakeSecrets, fakeClient, gitSourceSecrets)
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, fakeSecrets)
	setPrimaryDomain(t, db)
	seedApp(t, db, "web")

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

	if !fakeClient.createHookCall {
		t.Fatal("CreateRepoWebhook was not called")
	}
	if !strings.HasSuffix(fakeClient.createHookURL, "/api/v1/webhooks/github/web") {
		t.Errorf("createHookURL = %q, want it to end with the generic git-push webhook path", fakeClient.createHookURL)
	}
	storedSecret, err := gitSourceSecrets.Resolve(context.Background(), store.GitSourceSecretsKey("web"), gitSourceSecretKey)
	if err != nil {
		t.Fatalf("resolve stored git source webhook secret: %v", err)
	}
	if fakeClient.createHookSec != storedSecret {
		t.Errorf("createHookSec = %q, want it to match the stored git-source webhook secret %q", fakeClient.createHookSec, storedSecret)
	}
}

// TestHandleUseGitHubRepoAsSource_PermissionDenied is the test proving
// the actual degrade path this task cared most about: an installation
// whose CreateRepoWebhook call fails with githubapp.ErrPermissionDenied
// (the "installed before repository_hooks:write was requested" case)
// still ends up with a connected git source, a 2xx response, and a
// webhook_error explaining the manual fallback, not a failed request.
func TestHandleUseGitHubRepoAsSource_PermissionDenied(t *testing.T) {
	fakeSecrets := newFakeGitHubAppSecrets()
	fakeClient := &fakeGitHubAppClient{
		repo:          githubapp.Repo{FullName: "acme/web", DefaultBranch: "main"},
		createHookErr: githubapp.ErrPermissionDenied,
	}
	gitSourceSecrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitHubAndGitSource(t, fakeSecrets, fakeClient, gitSourceSecrets)
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, fakeSecrets)
	setPrimaryDomain(t, db)
	seedApp(t, db, "web")

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
	if _, err := db.GetGitSource(context.Background(), "web"); err != nil {
		t.Errorf("GetGitSource() error = %v, want the git source to remain connected despite the permission error", err)
	}
}

// TestHandleUseGitHubRepoAsSource_OtherWebhookFailure proves a
// non-permission webhook registration failure still fails the request
// (502), the same "connect first, webhook second, but a bad failure is
// still a failure" shape GitLab/Bitbucket's own use-as-source handlers
// have: only the specific, structural permission-denied case degrades.
func TestHandleUseGitHubRepoAsSource_OtherWebhookFailure(t *testing.T) {
	fakeSecrets := newFakeGitHubAppSecrets()
	fakeClient := &fakeGitHubAppClient{
		repo:          githubapp.Repo{FullName: "acme/web", DefaultBranch: "main"},
		createHookErr: errFakeSecretNotSet,
	}
	rt, db := newTestRouterWithGitHubAndGitSource(t, fakeSecrets, fakeClient, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, fakeSecrets)
	setPrimaryDomain(t, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/github-app/repos/acme/web/use-as-source", `{"app_name":"web"}`))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body = %s", rec.Code, rec.Body.String())
	}

	if _, err := db.GetGitSource(context.Background(), "web"); err != nil {
		t.Errorf("GetGitSource() error = %v, want the git source to remain connected despite the webhook failure", err)
	}
}

func TestGitHubAppUseAsSourceRoute_PlainWriteTokenForbidden(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	ctx := context.Background()

	const plaintext = "write-only-token-github" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write_gh", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite},
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/github-app/repos/acme/web/use-as-source", strings.NewReader(`{"app_name":"web"}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (AbilityWrite must not reach an AbilityWriteSensitive route)", rec.Code)
	}
}
