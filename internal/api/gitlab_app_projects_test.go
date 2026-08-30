package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// newTestRouterWithGitLabAndGitSource wires both a GitLab connection and
// git-source secrets into one router: handleUseGitLabProjectAsSource
// needs both, the same "reuses the exact same git-source hookup" point
// this test file exercises.
func newTestRouterWithGitLabAndGitSource(t *testing.T, gitlabSecrets GitLabAppSecrets, gitlabClient GitLabAppClient, gitSourceSecrets GitSourceSecrets) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db,
		WithGitLabAppSecrets(gitlabSecrets),
		WithGitSourceSecrets(gitSourceSecrets),
	)
	rt.gitlabAppClient = gitlabClient
	return rt, db
}

func authorizedGitLabSecrets(t *testing.T) *fakeGitLabAppSecrets {
	t.Helper()
	s := newFakeGitLabAppSecrets()
	key := store.GitLabAppSecretsKey()
	mustSetGitLabSecret(t, s, key, gitLabAppClientSecretKey, "csecret")
	mustSetGitLabSecret(t, s, key, gitLabAppAccessTokenKey, "at-1")
	mustSetGitLabSecret(t, s, key, gitLabAppTokenExpiresKey, "99999999999")
	return s
}

func TestHandleListGitLabAppProjects_NotAuthorized(t *testing.T) {
	rt, db := newTestRouterWithGitLabApp(t, newFakeGitLabAppSecrets(), &fakeGitLabAppClient{})
	cookie := loginTestSession(t, rt, db)
	seedGitLabAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitlab-app/projects", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListGitLabAppProjects_Success(t *testing.T) {
	fakeClient := &fakeGitLabAppClient{
		projects: []gitlabapp.Project{
			{ID: 1, Name: "widgets", PathWithNamespace: "acme/widgets", HTTPURLToRepo: "https://gitlab.example.com/acme/widgets.git", DefaultBranch: "main", Visibility: "private"},
		},
	}
	secrets := authorizedGitLabSecrets(t)
	rt, db := newTestRouterWithGitLabApp(t, secrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedGitLabAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitlab-app/projects", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"path_with_namespace":"acme/widgets"`) {
		t.Errorf("body = %s, want the project's path_with_namespace", rec.Body.String())
	}
}

func TestHandleListGitLabAppBranches_NotAuthorized(t *testing.T) {
	rt, db := newTestRouterWithGitLabApp(t, newFakeGitLabAppSecrets(), &fakeGitLabAppClient{})
	cookie := loginTestSession(t, rt, db)
	seedGitLabAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitlab-app/projects/1/branches", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListGitLabAppBranches_InvalidProjectID(t *testing.T) {
	secrets := authorizedGitLabSecrets(t)
	rt, db := newTestRouterWithGitLabApp(t, secrets, &fakeGitLabAppClient{})
	cookie := loginTestSession(t, rt, db)
	seedGitLabAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitlab-app/projects/not-a-number/branches", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListGitLabAppBranches_Success(t *testing.T) {
	fakeClient := &fakeGitLabAppClient{
		branches: []gitlabapp.Branch{
			{Name: "main", CommitSHA: "abc123"},
			{Name: "dev", CommitSHA: "def456"},
		},
	}
	secrets := authorizedGitLabSecrets(t)
	rt, db := newTestRouterWithGitLabApp(t, secrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedGitLabAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitlab-app/projects/1/branches", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"name":"main"`) || !strings.Contains(body, `"name":"dev"`) {
		t.Errorf("body = %s, missing expected branches", body)
	}
}

func TestHandleListGitLabAppBranches_UpstreamListError(t *testing.T) {
	fakeClient := &fakeGitLabAppClient{branchesErr: errFakeSecretNotSet}
	secrets := authorizedGitLabSecrets(t)
	rt, db := newTestRouterWithGitLabApp(t, secrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedGitLabAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/gitlab-app/projects/1/branches", ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body = %s", rec.Code, rec.Body.String())
	}
}

func TestGitLabAppProjectRoutes_PlainReadTokenForbidden(t *testing.T) {
	rt, db := newTestRouterWithGitLabApp(t, newFakeGitLabAppSecrets(), &fakeGitLabAppClient{})
	ctx := context.Background()

	const plaintext = "read-only-token-gitlab" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_read_gl", Name: "reader", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead},
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/gitlab-app/projects", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (AbilityRead must not reach an AbilityReadSensitive route)", rec.Code)
	}
}

func TestHandleUseGitLabProjectAsSource_AppNotFound(t *testing.T) {
	secrets := authorizedGitLabSecrets(t)
	rt, db := newTestRouterWithGitLabAndGitSource(t, secrets, &fakeGitLabAppClient{}, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedGitLabAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/gitlab-app/projects/1/use-as-source", `{"app_name":"missing"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUseGitLabProjectAsSource_InvalidProjectID(t *testing.T) {
	secrets := authorizedGitLabSecrets(t)
	rt, db := newTestRouterWithGitLabAndGitSource(t, secrets, &fakeGitLabAppClient{}, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/gitlab-app/projects/not-a-number/use-as-source", `{"app_name":"web"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

// TestHandleUseGitLabProjectAsSource_Success proves this handler ends up
// creating the exact same store.GitSource row (and webhook secret under
// the exact same GitSourceSecretsKey) that PUT .../git-source itself
// creates, and registers a GitLab project webhook pointed at that
// source's own webhook URL with that same secret.
func TestHandleUseGitLabProjectAsSource_Success(t *testing.T) {
	fakeClient := &fakeGitLabAppClient{
		project: gitlabapp.Project{
			ID: 7, Name: "web", PathWithNamespace: "acme/web",
			HTTPURLToRepo: "https://gitlab.example.com/acme/web.git", DefaultBranch: "trunk",
		},
	}
	secrets := authorizedGitLabSecrets(t)
	gitSourceSecrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitLabAndGitSource(t, secrets, fakeClient, gitSourceSecrets)
	cookie := loginTestSession(t, rt, db)
	seedGitLabAppConnection(t, db)
	setPrimaryDomain(t, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/gitlab-app/projects/7/use-as-source", `{"app_name":"web"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"repo_url":"https://gitlab.example.com/acme/web.git"`) {
		t.Errorf("body = %s, want repo_url from the looked-up project", body)
	}
	if !strings.Contains(body, `"branch":"trunk"`) {
		t.Errorf("body = %s, want branch defaulted from the project's own default_branch", body)
	}
	if !strings.Contains(body, `"webhook_secret"`) {
		t.Errorf("body = %s, want webhook_secret on first connect", body)
	}

	saved, err := db.GetGitSource(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetGitSource() error = %v", err)
	}
	if saved.RepoURL != "https://gitlab.example.com/acme/web.git" {
		t.Errorf("saved git source repo_url = %q, want the gitlab project's clone url", saved.RepoURL)
	}

	if !fakeClient.createHookCall {
		t.Fatal("CreateProjectWebhook was not called")
	}
	if !strings.HasSuffix(fakeClient.createHookURL, "/api/v1/webhooks/github/web") {
		t.Errorf("createHookURL = %q, want it to end with the generic git-push webhook path", fakeClient.createHookURL)
	}
	storedSecret, err := gitSourceSecrets.Resolve(context.Background(), store.GitSourceSecretsKey("web"), gitSourceSecretKey)
	if err != nil {
		t.Fatalf("resolve stored git source webhook secret: %v", err)
	}
	if fakeClient.createHookTok != storedSecret {
		t.Errorf("createHookTok = %q, want it to match the stored git-source webhook secret %q", fakeClient.createHookTok, storedSecret)
	}
}

func TestHandleUseGitLabProjectAsSource_WebhookRegistrationFails(t *testing.T) {
	fakeClient := &fakeGitLabAppClient{
		project:       gitlabapp.Project{ID: 7, HTTPURLToRepo: "https://gitlab.example.com/acme/web.git", DefaultBranch: "main"},
		createHookErr: errFakeSecretNotSet,
	}
	secrets := authorizedGitLabSecrets(t)
	rt, db := newTestRouterWithGitLabAndGitSource(t, secrets, fakeClient, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedGitLabAppConnection(t, db)
	setPrimaryDomain(t, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/gitlab-app/projects/7/use-as-source", `{"app_name":"web"}`))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body = %s", rec.Code, rec.Body.String())
	}

	// The git source is still connected even though the webhook
	// registration failed: this handler doesn't roll back a partial
	// success, matching the reconciler-adjacent "connect first, webhook
	// second" doc comment on handleUseGitLabProjectAsSource.
	if _, err := db.GetGitSource(context.Background(), "web"); err != nil {
		t.Errorf("GetGitSource() error = %v, want the git source to remain connected despite the webhook failure", err)
	}
}
