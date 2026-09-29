package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dockerhub"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeDockerHubClient is a hand-written fake for DockerHubClient, the
// same "table-driven, fail on demand" shape fakeRegistryCatalogClient
// already establishes for the analogous built-in-registry handlers.
type fakeDockerHubClient struct {
	repos     []dockerhub.Repository
	searchErr error
	tags      map[string][]dockerhub.Tag
	tagsErr   error
	gotQuery  string
	gotNS     string
	gotRepo   string
}

func (f *fakeDockerHubClient) SearchRepositories(_ context.Context, query string, _ int) ([]dockerhub.Repository, error) {
	f.gotQuery = query
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.repos, nil
}

func (f *fakeDockerHubClient) ListTags(_ context.Context, namespace, repository string, _ int) ([]dockerhub.Tag, error) {
	f.gotNS, f.gotRepo = namespace, repository
	if f.tagsErr != nil {
		return nil, f.tagsErr
	}
	return f.tags[namespace+"/"+repository], nil
}

func newTestRouterWithDockerHub(t *testing.T, client *fakeDockerHubClient) (*Router, *store.DB) {
	t.Helper()
	rt, db := newTestRouter(t)
	rt.dockerHubClient = client
	return rt, db
}

func TestHandleDockerHubSearch_MissingQuery(t *testing.T) {
	client := &fakeDockerHubClient{}
	rt, db := newTestRouterWithDockerHub(t, client)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/dockerhub/search", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleDockerHubSearch_Success(t *testing.T) {
	client := &fakeDockerHubClient{repos: []dockerhub.Repository{
		{Name: "postgres", ShortDescription: "The PostgreSQL database", StarCount: 12000, IsOfficial: true},
		{Name: "bitnami/postgresql", ShortDescription: "Bitnami postgres", StarCount: 300},
	}}
	rt, db := newTestRouterWithDockerHub(t, client)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/dockerhub/search?q=postgres", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got dockerHubSearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Results) != 2 || got.Results[0].Name != "postgres" {
		t.Errorf("Results = %+v, want 2 results starting with postgres", got.Results)
	}
	if client.gotQuery != "postgres" {
		t.Errorf("client saw query = %q, want postgres", client.gotQuery)
	}
}

func TestHandleDockerHubSearch_UpstreamError(t *testing.T) {
	client := &fakeDockerHubClient{searchErr: errors.New("rate limited")}
	rt, db := newTestRouterWithDockerHub(t, client)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/dockerhub/search?q=postgres", ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
}

func TestHandleDockerHubTags_Success(t *testing.T) {
	client := &fakeDockerHubClient{tags: map[string][]dockerhub.Tag{
		"library/postgres": {{Name: "16"}, {Name: "latest"}},
	}}
	rt, db := newTestRouterWithDockerHub(t, client)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/dockerhub/repositories/library/postgres/tags", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got dockerHubTagsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Namespace != "library" || got.Repository != "postgres" || len(got.Tags) != 2 {
		t.Errorf("got = %+v, want namespace=library repository=postgres with 2 tags", got)
	}
	if client.gotNS != "library" || client.gotRepo != "postgres" {
		t.Errorf("client saw namespace/repo = %q/%q, want library/postgres", client.gotNS, client.gotRepo)
	}
}

func TestHandleDockerHubTags_NotFound(t *testing.T) {
	client := &fakeDockerHubClient{tagsErr: dockerhub.ErrNotFound}
	rt, db := newTestRouterWithDockerHub(t, client)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/dockerhub/repositories/library/ghost/tags", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleDockerHubTags_GenericUpstreamError(t *testing.T) {
	client := &fakeDockerHubClient{tagsErr: errors.New("connection reset")}
	rt, db := newTestRouterWithDockerHub(t, client)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/dockerhub/repositories/library/postgres/tags", ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
}

func TestDockerHubRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []string{
		"/api/v1/dockerhub/search?q=postgres",
		"/api/v1/dockerhub/repositories/library/postgres/tags",
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
