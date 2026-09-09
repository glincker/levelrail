package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/registrycatalog"
	"github.com/GLINCKER/levelrail/internal/store"
)

// newTestRouterWithRegistryCredentialBrowse wires a fresh Router with a
// registry-credential secrets setter (needed both to create the fixture
// credential via the API and to resolve its password on browse) plus the
// given fake catalog client, reusing fakeRegistryCatalogClient from
// registry_catalog_test.go: the same underlying RegistryCatalogClient
// interface, just pointed at a credential's own RegistryHost instead of
// the built-in registry's loopback address.
func newTestRouterWithRegistryCredentialBrowse(t *testing.T, catalog *fakeRegistryCatalogClient, secretsSetter RegistryCredentialSecretsSetter) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := discardLogger()
	rt := NewRouter(logger, testBrand(), db, WithRegistryCredentialSecrets(secretsSetter))
	rt.registryCatalog = catalog
	return rt, db
}

// newRegistryCredentialBrowseFixture wires a router with the given fake
// catalog client/secrets setter, logs in, and creates the one credential
// every browse test below exercises, cutting the repeated setup block
// SonarCloud flagged as duplication across these table-shaped tests.
func newRegistryCredentialBrowseFixture(t *testing.T, client *fakeRegistryCatalogClient, setter *fakeRegistryCredentialSecretsSetter) (*Router, *http.Cookie, registryCredentialResource) {
	t.Helper()
	rt, db := newTestRouterWithRegistryCredentialBrowse(t, client, setter)
	cookie := loginTestSession(t, rt, db)
	created := createTestRegistryCredential(t, rt, cookie, `{"name":"ghcr-bot","registry_host":"ghcr.io","username":"bot","password":"tok"}`)
	return rt, cookie, created
}

func TestHandleListRegistryCredentialRepositories_NotConfigured(t *testing.T) {
	rt, cookie, created := newRegistryCredentialBrowseFixture(t, &fakeRegistryCatalogClient{}, &fakeRegistryCredentialSecretsSetter{})
	rt.registryCatalog = nil // simulates a control plane wiring gap, distinct from "no master key"

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-credentials/"+created.ID+"/repositories", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleListRegistryCredentialRepositories_CredentialNotFound(t *testing.T) {
	rt, cookie, _ := newRegistryCredentialBrowseFixture(t, &fakeRegistryCatalogClient{}, &fakeRegistryCredentialSecretsSetter{})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-credentials/regcred_missing/repositories", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleListRegistryCredentialRepositories_Success(t *testing.T) {
	client := &fakeRegistryCatalogClient{repos: []string{"alpha", "beta"}}
	rt, cookie, created := newRegistryCredentialBrowseFixture(t, client, &fakeRegistryCredentialSecretsSetter{resolveValue: "hunter2"})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-credentials/"+created.ID+"/repositories", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got registryRepositoriesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Repositories) != 2 || got.Repositories[0] != "alpha" || got.Repositories[1] != "beta" {
		t.Errorf("Repositories = %v, want [alpha beta]", got.Repositories)
	}
	if client.gotBase != "ghcr.io" || client.gotUser != "bot" || client.gotPass != "hunter2" {
		t.Errorf("client saw base/user/pass = %q/%q/%q, want ghcr.io/bot/hunter2", client.gotBase, client.gotUser, client.gotPass)
	}
}

func TestHandleListRegistryCredentialRepositories_UpstreamError(t *testing.T) {
	client := &fakeRegistryCatalogClient{repoErr: errors.New("connection refused")}
	rt, cookie, created := newRegistryCredentialBrowseFixture(t, client, &fakeRegistryCredentialSecretsSetter{resolveValue: "tok"})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-credentials/"+created.ID+"/repositories", ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
}

func TestHandleListRegistryCredentialTags_MissingRepositoryParam(t *testing.T) {
	rt, cookie, created := newRegistryCredentialBrowseFixture(t, &fakeRegistryCatalogClient{}, &fakeRegistryCredentialSecretsSetter{})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-credentials/"+created.ID+"/tags", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleListRegistryCredentialTags_CredentialNotFound(t *testing.T) {
	rt, cookie, _ := newRegistryCredentialBrowseFixture(t, &fakeRegistryCatalogClient{}, &fakeRegistryCredentialSecretsSetter{})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-credentials/regcred_missing/tags?repository=myapp", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleListRegistryCredentialTags_Success(t *testing.T) {
	client := &fakeRegistryCatalogClient{tags: map[string][]string{"myapp": {"v1", "latest"}}}
	rt, cookie, created := newRegistryCredentialBrowseFixture(t, client, &fakeRegistryCredentialSecretsSetter{resolveValue: "tok"})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-credentials/"+created.ID+"/tags?repository=myapp", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got registryTagsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Repository != "myapp" || len(got.Tags) != 2 {
		t.Errorf("got = %+v, want repository myapp with 2 tags", got)
	}
	if client.gotRepos != "myapp" {
		t.Errorf("client saw repository = %q, want myapp", client.gotRepos)
	}
}

func TestHandleListRegistryCredentialTags_RepositoryNotFound(t *testing.T) {
	client := &fakeRegistryCatalogClient{tagsErr: registrycatalog.ErrNotFound}
	rt, cookie, created := newRegistryCredentialBrowseFixture(t, client, &fakeRegistryCredentialSecretsSetter{resolveValue: "tok"})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-credentials/"+created.ID+"/tags?repository=ghost", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleListRegistryCredentialTags_UpstreamError(t *testing.T) {
	client := &fakeRegistryCatalogClient{tagsErr: errors.New("connection refused")}
	rt, cookie, created := newRegistryCredentialBrowseFixture(t, client, &fakeRegistryCredentialSecretsSetter{resolveValue: "tok"})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-credentials/"+created.ID+"/tags?repository=myapp", ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
}

func TestRegistryCredentialBrowseRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouterWithRegistryCredentialBrowse(t, &fakeRegistryCatalogClient{}, &fakeRegistryCredentialSecretsSetter{})

	assertProviderRoutesRequireAuth(t, rt, []providerRouteCase{
		{method: http.MethodGet, path: "/api/v1/registry-credentials/regcred_x/repositories"},
		{method: http.MethodGet, path: "/api/v1/registry-credentials/regcred_x/tags?repository=myapp"},
	})
}

// TestRegistryCredentialBrowseRoutes_PlainReadTokenForbidden proves both
// routes sit at AbilityReadSensitive, not the plain AbilityRead tier
// handleGetRegistryCredential itself uses: reading a browsed repository
// list only works because a stored credential's secret is resolved
// server-side, the same reasoning GET /api/v1/git-providers and the
// github-app/gitlab-app/bitbucket-app repo-browsing routes already
// establish for this tier.
func TestRegistryCredentialBrowseRoutes_PlainReadTokenForbidden(t *testing.T) {
	rt, db := newTestRouterWithRegistryCredentialBrowse(t, &fakeRegistryCatalogClient{}, &fakeRegistryCredentialSecretsSetter{})

	const plaintext = "read-only-token-registry-browse" //nolint:gosec // fake fixture, not a real credential
	assertProviderRoutesForbiddenForAbilities(t, rt, db, "tok_read_registry_browse", plaintext, []string{AbilityRead}, []providerRouteCase{
		{method: http.MethodGet, path: "/api/v1/registry-credentials/regcred_x/repositories"},
		{method: http.MethodGet, path: "/api/v1/registry-credentials/regcred_x/tags?repository=myapp"},
	})
}
