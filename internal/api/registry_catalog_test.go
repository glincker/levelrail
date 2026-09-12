package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/registrycatalog"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeRegistryCatalogClient is a hand-written fake for RegistryCatalogClient,
// the same "table-driven, fail on demand" shape fakeDockerRuntime and
// friends already establish elsewhere in this package.
type fakeRegistryCatalogClient struct {
	repos    []string
	repoErr  error
	tags     map[string][]string
	tagsErr  error
	gotBase  string
	gotUser  string
	gotPass  string
	gotRepos string
}

func (f *fakeRegistryCatalogClient) ListRepositories(_ context.Context, baseURL, username, password string) ([]string, error) {
	f.gotBase, f.gotUser, f.gotPass = baseURL, username, password
	if f.repoErr != nil {
		return nil, f.repoErr
	}
	return f.repos, nil
}

func (f *fakeRegistryCatalogClient) ListTags(_ context.Context, baseURL, username, password, repository string) ([]string, error) {
	f.gotBase, f.gotUser, f.gotPass, f.gotRepos = baseURL, username, password, repository
	if f.tagsErr != nil {
		return nil, f.tagsErr
	}
	return f.tags[repository], nil
}

// fakeRegistryCatalogSecrets is a hand-written fake for
// RegistryCatalogSecrets, resolving to secrets.ErrValueNotFound (the real
// Manager's own sentinel for "nothing was ever set") when no password was
// configured, the same signal handleListRegistryRepositories/Tags branch
// on.
type fakeRegistryCatalogSecrets struct {
	password string
	set      bool
	err      error
}

func (f *fakeRegistryCatalogSecrets) Resolve(_ context.Context, _, _ string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if !f.set {
		return "", secrets.ErrValueNotFound
	}
	return f.password, nil
}

// newTestRouterWithRegistryCatalog wires a fresh Router with an enabled,
// credentialed built-in registry plus the given fake catalog client, the
// common setup every success-path test below starts from.
func newTestRouterWithRegistryCatalog(t *testing.T, client *fakeRegistryCatalogClient, catalogSecrets RegistryCatalogSecrets) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := discardLogger()
	rt := NewRouter(logger, testBrand(), db, WithRegistryCatalogSecrets(catalogSecrets))
	rt.registryCatalog = client
	return rt, db
}

func enableRegistry(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.UpdateRegistrySettings(context.Background(), store.RegistrySettings{
		Enabled: true, Host: "registry.example", Username: "levelrail",
	}); err != nil {
		t.Fatalf("UpdateRegistrySettings() error = %v", err)
	}
}

func TestHandleListRegistryRepositories_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no catalog secrets, no client override
	rt.registryCatalog = nil
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry/repositories", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleListRegistryRepositories_NotEnabled(t *testing.T) {
	client := &fakeRegistryCatalogClient{}
	rt, db := newTestRouterWithRegistryCatalog(t, client, &fakeRegistryCatalogSecrets{password: "pw", set: true})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry/repositories", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleListRegistryRepositories_NoCredentialsYet(t *testing.T) {
	client := &fakeRegistryCatalogClient{}
	rt, db := newTestRouterWithRegistryCatalog(t, client, &fakeRegistryCatalogSecrets{set: false})
	enableRegistry(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry/repositories", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleListRegistryRepositories_Success(t *testing.T) {
	client := &fakeRegistryCatalogClient{repos: []string{"alpha", "beta"}}
	rt, db := newTestRouterWithRegistryCatalog(t, client, &fakeRegistryCatalogSecrets{password: "hunter2", set: true})
	enableRegistry(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry/repositories", ""))
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
	if client.gotUser != "levelrail" || client.gotPass != "hunter2" {
		t.Errorf("client saw user/pass = %q/%q, want levelrail/hunter2", client.gotUser, client.gotPass)
	}
}

func TestHandleListRegistryRepositories_UpstreamError(t *testing.T) {
	client := &fakeRegistryCatalogClient{repoErr: errors.New("connection refused")}
	rt, db := newTestRouterWithRegistryCatalog(t, client, &fakeRegistryCatalogSecrets{password: "pw", set: true})
	enableRegistry(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry/repositories", ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
}

func TestHandleListRegistryTags_MissingRepositoryParam(t *testing.T) {
	client := &fakeRegistryCatalogClient{}
	rt, db := newTestRouterWithRegistryCatalog(t, client, &fakeRegistryCatalogSecrets{password: "pw", set: true})
	enableRegistry(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry/tags", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleListRegistryTags_Success(t *testing.T) {
	client := &fakeRegistryCatalogClient{tags: map[string][]string{"myapp": {"v1", "latest"}}}
	rt, db := newTestRouterWithRegistryCatalog(t, client, &fakeRegistryCatalogSecrets{password: "pw", set: true})
	enableRegistry(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry/tags?repository=myapp", ""))
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

func TestHandleListRegistryTags_RepositoryNotFound(t *testing.T) {
	client := &fakeRegistryCatalogClient{tagsErr: registrycatalog.ErrNotFound}
	rt, db := newTestRouterWithRegistryCatalog(t, client, &fakeRegistryCatalogSecrets{password: "pw", set: true})
	enableRegistry(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry/tags?repository=ghost", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleListRegistryTags_GenericUpstreamError(t *testing.T) {
	// Proves handleListRegistryTags only special-cases the exact
	// registrycatalog.ErrNotFound sentinel (via errors.Is) and treats
	// every other upstream failure as a generic 502, not by
	// string-matching an error message.
	client := &fakeRegistryCatalogClient{tagsErr: errors.New("connection reset")}
	rt, db := newTestRouterWithRegistryCatalog(t, client, &fakeRegistryCatalogSecrets{password: "pw", set: true})
	enableRegistry(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry/tags?repository=myapp", ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
}

func TestRegistryCatalogRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []string{
		"/api/v1/registry/repositories",
		"/api/v1/registry/tags?repository=myapp",
	}
	for _, target := range routes {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}
