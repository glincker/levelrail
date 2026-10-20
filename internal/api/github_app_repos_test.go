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

func TestHandleListGitHubAppRepos_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithGitHubAppSecrets
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/repos", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListGitHubAppRepos_NotConnected(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/repos", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListGitHubAppRepos_NotInstalled(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveGitHubAppConnection(context.Background(), store.GitHubAppConnection{
		AppID: 42, ClientID: "Iv1.abc", InstanceURL: "https://github.com", CreatedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/repos", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (connected but not installed), body = %s", rec.Code, rec.Body.String())
	}
}

func seedInstalledGitHubApp(t *testing.T, db *store.DB, fakeSecrets *fakeGitHubAppSecrets) {
	t.Helper()
	pemKey := testRSAPrivateKeyPEM(t)
	if err := fakeSecrets.SetValue(context.Background(), store.GitHubAppSecretsKey(), "private_key", pemKey); err != nil {
		t.Fatalf("seed private_key: %v", err)
	}
	if err := db.SaveGitHubAppConnection(context.Background(), store.GitHubAppConnection{
		AppID: 42, ClientID: "Iv1.abc", InstanceURL: "https://github.com", CreatedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	if err := db.UpdateGitHubAppInstallation(context.Background(), 7, "octocat"); err != nil {
		t.Fatalf("seed installation: %v", err)
	}
}

func TestHandleListGitHubAppRepos_Success(t *testing.T) {
	fakeSecrets := newFakeGitHubAppSecrets()
	fakeClient := &fakeGitHubAppClient{
		mintToken: githubapp.InstallationToken{Token: "ghs_installtoken"},
		repos: []githubapp.Repo{
			{FullName: "acme/widgets", Name: "widgets", OwnerLogin: "acme", Private: true, DefaultBranch: "main"},
		},
	}
	rt, db := newTestRouterWithGitHubApp(t, fakeSecrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, fakeSecrets)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/repos", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"full_name":"acme/widgets"`) {
		t.Errorf("body = %s, missing expected repo", body)
	}
	if !strings.Contains(body, `"clone_url":"https://github.com/acme/widgets.git"`) {
		t.Errorf("body = %s, clone_url not built as expected", body)
	}
	if fakeClient.gotInstallID != 7 {
		t.Errorf("MintInstallationToken called with installationID = %d, want 7", fakeClient.gotInstallID)
	}
	// The installation token itself must never leak into the response.
	if strings.Contains(body, "ghs_installtoken") {
		t.Errorf("body contains the installation access token, must never be echoed back: %s", body)
	}
}

func TestHandleListGitHubAppRepos_MintTokenError(t *testing.T) {
	fakeSecrets := newFakeGitHubAppSecrets()
	fakeClient := &fakeGitHubAppClient{mintErr: errFakeSecretNotSet}
	rt, db := newTestRouterWithGitHubApp(t, fakeSecrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, fakeSecrets)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/repos", ""))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListGitHubAppRepos_UpstreamListError(t *testing.T) {
	fakeSecrets := newFakeGitHubAppSecrets()
	fakeClient := &fakeGitHubAppClient{
		mintToken: githubapp.InstallationToken{Token: "ghs_installtoken"},
		reposErr:  errFakeSecretNotSet,
	}
	rt, db := newTestRouterWithGitHubApp(t, fakeSecrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, fakeSecrets)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/repos", ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListGitHubAppBranches_MissingOwnerOrRepo(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)

	// The mux pattern requires both {owner} and {repo} segments, so a
	// request that omits one simply won't match this route at all
	// (404 from the mux itself), which is exactly the "clean rejection,
	// not a panic" property this test wants to confirm holds end to end.
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/repos//branches", ""))
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("status = %d, want a clean 4xx, not a panic/500", rec.Code)
	}
}

func TestHandleListGitHubAppBranches_Success(t *testing.T) {
	fakeSecrets := newFakeGitHubAppSecrets()
	fakeClient := &fakeGitHubAppClient{
		mintToken: githubapp.InstallationToken{Token: "ghs_installtoken"},
		branches: []githubapp.Branch{
			{Name: "main", CommitSHA: "abc123"},
			{Name: "dev", CommitSHA: "def456"},
		},
	}
	rt, db := newTestRouterWithGitHubApp(t, fakeSecrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, fakeSecrets)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/repos/acme/widgets/branches", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"name":"main"`) || !strings.Contains(body, `"name":"dev"`) {
		t.Errorf("body = %s, missing expected branches", body)
	}
}

func TestHandleListGitHubAppBranches_NotConnected(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/repos/acme/widgets/branches", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}
