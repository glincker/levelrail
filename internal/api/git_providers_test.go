package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TestHandleListGitProviders_NoneConnected proves the endpoint responds
// with all three providers reported as not connected rather than
// erroring, when nothing is configured at all: the default state for a
// fresh control plane.
func TestHandleListGitProviders_NoneConnected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/git-providers", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, provider := range []string{"github", "gitlab", "bitbucket"} {
		if !strings.Contains(body, `"provider":"`+provider+`"`) {
			t.Errorf("body = %s, missing %q entry", body, provider)
		}
	}
	if strings.Contains(body, `"connected":true`) {
		t.Errorf("body = %s, want every provider connected:false", body)
	}
}

// TestHandleListGitProviders_PlainReadTokenAllowed proves this endpoint
// sits at AbilityReadSensitive, not AbilityRoot like the three
// per-provider status endpoints: an AbilityReadSensitive token (not
// merely AbilityRead) must reach it, exactly the tier the aggregated
// endpoint exists to unblock for a non-root deploy-scoped user.
func TestHandleListGitProviders_PlainReadTokenForbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	const plaintext = "read-only-token-providers" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_read_providers", Name: "reader", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead},
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-providers", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (AbilityRead must not reach an AbilityReadSensitive route)", rec.Code)
	}
}

func TestHandleListGitProviders_ReadSensitiveTokenAllowed(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	const plaintext = "sensitive-token-providers" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_sensitive_providers", Name: "sensitive", TokenHash: hashToken(plaintext), Abilities: []string{AbilityReadSensitive},
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-providers", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
}

// TestHandleListGitProviders_GitHubConnectedNotInstalled proves a
// GitHub App connection with no installation yet still reports
// connected:false, since none of the picker's capabilities (repo/branch
// listing, webhook registration) work until an installation exists.
func TestHandleListGitProviders_GitHubConnectedNotInstalled(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveGitHubAppConnection(context.Background(), store.GitHubAppConnection{
		AppID: 42, ClientID: "Iv1.abc", InstanceURL: "https://github.com", CreatedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/git-providers", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `{"provider":"github","connected":false`) {
		t.Errorf("body = %s, want github connected:false (installed missing)", rec.Body.String())
	}
}

// TestHandleListGitProviders_AllConnected proves every capability flag
// flips to true once each provider is fully connected (installed for
// GitHub, authorized for GitLab/Bitbucket), and that GitHub alone
// reports can_auth_clone:true.
func TestHandleListGitProviders_AllConnected(t *testing.T) {
	fakeGitHubSecrets := newFakeGitHubAppSecrets()
	fakeGitLabSecrets := authorizedGitLabSecrets(t)
	fakeBitbucketSecrets := authorizedBitbucketSecrets(t)

	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db,
		WithGitHubAppSecrets(fakeGitHubSecrets),
		WithGitLabAppSecrets(fakeGitLabSecrets),
		WithBitbucketAppSecrets(fakeBitbucketSecrets),
	)
	rt.githubAppClient = &fakeGitHubAppClient{}
	rt.gitlabAppClient = &fakeGitLabAppClient{}
	rt.bitbucketAppClient = &fakeBitbucketAppClient{}
	cookie := loginTestSession(t, rt, db)

	seedInstalledGitHubApp(t, db, fakeGitHubSecrets)
	seedGitLabAppConnection(t, db)
	seedBitbucketAppConnection(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/git-providers", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `{"provider":"github","connected":true,"can_list_branches":true,"can_register_webhook":true,"can_auth_clone":true}`) {
		t.Errorf("body = %s, want github fully capable", body)
	}
	if !strings.Contains(body, `{"provider":"gitlab","connected":true,"can_list_branches":true,"can_register_webhook":true,"can_auth_clone":false}`) {
		t.Errorf("body = %s, want gitlab connected with branch listing and webhook registration, no auth clone", body)
	}
	if !strings.Contains(body, `{"provider":"bitbucket","connected":true,"can_list_branches":true,"can_register_webhook":true,"can_auth_clone":false}`) {
		t.Errorf("body = %s, want bitbucket connected with branch listing and webhook registration, no auth clone", body)
	}
}
