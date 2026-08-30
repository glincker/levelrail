package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/bitbucketapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// newTestRouterWithBitbucketAndGitSource wires both a Bitbucket
// connection and git-source secrets into one router, the same
// "reuses the exact same git-source hookup" point
// newTestRouterWithGitLabAndGitSource's own doc comment establishes.
func newTestRouterWithBitbucketAndGitSource(t *testing.T, bitbucketSecrets BitbucketAppSecrets, bitbucketClient BitbucketAppClient, gitSourceSecrets GitSourceSecrets) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db,
		WithBitbucketAppSecrets(bitbucketSecrets),
		WithGitSourceSecrets(gitSourceSecrets),
	)
	rt.bitbucketAppClient = bitbucketClient
	return rt, db
}

func authorizedBitbucketSecrets(t *testing.T) *fakeBitbucketAppSecrets {
	t.Helper()
	s := newFakeBitbucketAppSecrets()
	key := store.BitbucketAppSecretsKey()
	mustSetBitbucketSecret(t, s, key, bitbucketAppSecretKey, "the-secret")
	mustSetBitbucketSecret(t, s, key, bitbucketAppAccessTokenKey, "at-1")
	mustSetBitbucketSecret(t, s, key, bitbucketAppTokenExpiresKey, "99999999999")
	return s
}

func TestHandleListBitbucketAppRepos_NotAuthorized(t *testing.T) {
	rt, db := newTestRouterWithBitbucketApp(t, newFakeBitbucketAppSecrets(), &fakeBitbucketAppClient{})
	cookie := loginTestSession(t, rt, db)
	seedBitbucketAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/bitbucket-app/repos", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListBitbucketAppRepos_Success(t *testing.T) {
	fakeClient := &fakeBitbucketAppClient{
		repos: []bitbucketapp.Repo{
			{FullName: "acme/widgets", Name: "widgets", CloneURL: "https://bitbucket.org/acme/widgets.git", DefaultBranch: "main", Private: true},
		},
	}
	secrets := authorizedBitbucketSecrets(t)
	rt, db := newTestRouterWithBitbucketApp(t, secrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedBitbucketAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/bitbucket-app/repos", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"full_name":"acme/widgets"`) {
		t.Errorf("body = %s, want the repo's full_name", rec.Body.String())
	}
}

func TestHandleListBitbucketAppBranches_Success(t *testing.T) {
	fakeClient := &fakeBitbucketAppClient{
		branches: []bitbucketapp.Branch{{Name: "main", CommitSHA: "abc123"}},
	}
	secrets := authorizedBitbucketSecrets(t)
	rt, db := newTestRouterWithBitbucketApp(t, secrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedBitbucketAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/bitbucket-app/repos/acme/widgets/branches", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"name":"main"`) {
		t.Errorf("body = %s, want the branch name", rec.Body.String())
	}
}

func TestBitbucketAppRepoRoutes_PlainReadTokenForbidden(t *testing.T) {
	rt, db := newTestRouterWithBitbucketApp(t, newFakeBitbucketAppSecrets(), &fakeBitbucketAppClient{})
	ctx := context.Background()

	const plaintext = "read-only-token-bitbucket" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_read_bb", Name: "reader", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead},
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bitbucket-app/repos", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (AbilityRead must not reach an AbilityReadSensitive route)", rec.Code)
	}
}

func TestHandleUseBitbucketRepoAsSource_AppNotFound(t *testing.T) {
	secrets := authorizedBitbucketSecrets(t)
	rt, db := newTestRouterWithBitbucketAndGitSource(t, secrets, &fakeBitbucketAppClient{}, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedBitbucketAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/bitbucket-app/repos/acme/widgets/use-as-source", `{"app_name":"missing"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

// TestHandleUseBitbucketRepoAsSource_Success proves this handler ends
// up creating the exact same store.GitSource row (and webhook secret
// under the exact same GitSourceSecretsKey) that PUT .../git-source
// itself creates, and registers a Bitbucket repo webhook pointed at
// that source's own webhook URL with that same secret, mirroring
// TestHandleUseGitLabProjectAsSource_Success.
func TestHandleUseBitbucketRepoAsSource_Success(t *testing.T) {
	fakeClient := &fakeBitbucketAppClient{
		repo: bitbucketapp.Repo{
			FullName: "acme/web", Name: "web",
			CloneURL: "https://bitbucket.org/acme/web.git", DefaultBranch: "trunk",
		},
	}
	secrets := authorizedBitbucketSecrets(t)
	gitSourceSecrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithBitbucketAndGitSource(t, secrets, fakeClient, gitSourceSecrets)
	cookie := loginTestSession(t, rt, db)
	seedBitbucketAppConnection(t, db)
	setPrimaryDomain(t, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/bitbucket-app/repos/acme/web/use-as-source", `{"app_name":"web"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"repo_url":"https://bitbucket.org/acme/web.git"`) {
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
	if saved.RepoURL != "https://bitbucket.org/acme/web.git" {
		t.Errorf("saved git source repo_url = %q, want the bitbucket repo's clone url", saved.RepoURL)
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
	if fakeClient.createHookTok != storedSecret {
		t.Errorf("createHookTok = %q, want it to match the stored git-source webhook secret %q", fakeClient.createHookTok, storedSecret)
	}
}

func TestHandleUseBitbucketRepoAsSource_WebhookRegistrationFails(t *testing.T) {
	fakeClient := &fakeBitbucketAppClient{
		repo:          bitbucketapp.Repo{FullName: "acme/web", CloneURL: "https://bitbucket.org/acme/web.git", DefaultBranch: "main"},
		createHookErr: errFakeSecretNotSet,
	}
	secrets := authorizedBitbucketSecrets(t)
	rt, db := newTestRouterWithBitbucketAndGitSource(t, secrets, fakeClient, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedBitbucketAppConnection(t, db)
	setPrimaryDomain(t, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/bitbucket-app/repos/acme/web/use-as-source", `{"app_name":"web"}`))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body = %s", rec.Code, rec.Body.String())
	}

	// The git source is still connected even though the webhook
	// registration failed: this handler doesn't roll back a partial
	// success, the same "connect first, webhook second" shape
	// handleUseGitLabProjectAsSource's own equivalent test documents.
	if _, err := db.GetGitSource(context.Background(), "web"); err != nil {
		t.Errorf("GetGitSource() error = %v, want the git source to remain connected despite the webhook failure", err)
	}
}
