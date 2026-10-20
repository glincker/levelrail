package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleListGitHubAppInstallations_Empty(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/installations", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var resp gitHubAppInstallationListResource
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Installations) != 0 {
		t.Errorf("Installations = %+v, want empty", resp.Installations)
	}
	if resp.AddOrgURL != "" {
		t.Errorf("AddOrgURL = %q, want empty (no slug recorded)", resp.AddOrgURL)
	}
}

func TestHandleListGitHubAppInstallations_WithDataAndSlug(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)

	slug := "my-app"
	if err := db.SaveGitHubAppConnection(context.Background(), store.GitHubAppConnection{
		AppID: 42, ClientID: "Iv1.abc", Slug: &slug, InstanceURL: "https://github.com", CreatedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	if err := db.UpsertGitHubAppInstallation(context.Background(), 111, "acme-corp", "organization"); err != nil {
		t.Fatalf("seed installation: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/installations", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	var resp gitHubAppInstallationListResource
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Installations) != 1 || resp.Installations[0].AccountLogin != "acme-corp" {
		t.Errorf("Installations = %+v, want one row for acme-corp", resp.Installations)
	}
	wantURL := "https://github.com/apps/my-app/installations/new"
	if resp.AddOrgURL != wantURL {
		t.Errorf("AddOrgURL = %q, want %q", resp.AddOrgURL, wantURL)
	}
}

func TestHandleDeleteGitHubAppInstallation_Success(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)

	if err := db.UpsertGitHubAppInstallation(context.Background(), 111, "acme-corp", "organization"); err != nil {
		t.Fatalf("seed installation: %v", err)
	}
	installations, err := db.ListGitHubAppInstallations(context.Background())
	if err != nil || len(installations) != 1 {
		t.Fatalf("ListGitHubAppInstallations() = %+v, %v, want exactly 1 row", installations, err)
	}

	rec := httptest.NewRecorder()
	path := fmt.Sprintf("/api/v1/github-app/installations/%d", installations[0].ID)
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, path, ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body = %s", rec.Code, rec.Body.String())
	}

	installations, err = db.ListGitHubAppInstallations(context.Background())
	if err != nil || len(installations) != 0 {
		t.Errorf("ListGitHubAppInstallations() after delete = %+v, %v, want empty", installations, err)
	}
}

func TestHandleDeleteGitHubAppInstallation_NotFound(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/github-app/installations/99999", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteGitHubAppInstallation_BlockedWhileInUse(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveGitHubAppConnection(context.Background(), store.GitHubAppConnection{
		AppID: 42, ClientID: "Iv1.abc", InstanceURL: "https://github.com", CreatedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	if err := db.UpsertGitHubAppInstallation(context.Background(), 111, "acme-corp", "organization"); err != nil {
		t.Fatalf("seed installation: %v", err)
	}
	if err := db.SaveGitSource(context.Background(), store.GitSource{
		ServiceName: "web", RepoURL: "https://github.com/acme-corp/web.git", Branch: "main", BuildType: "dockerfile",
	}); err != nil {
		t.Fatalf("seed git source: %v", err)
	}

	installations, err := db.ListGitHubAppInstallations(context.Background())
	if err != nil || len(installations) != 1 {
		t.Fatalf("ListGitHubAppInstallations() = %+v, %v, want exactly 1 row", installations, err)
	}

	rec := httptest.NewRecorder()
	path := fmt.Sprintf("/api/v1/github-app/installations/%d", installations[0].ID)
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, path, ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}

	installations, err = db.ListGitHubAppInstallations(context.Background())
	if err != nil || len(installations) != 1 {
		t.Errorf("ListGitHubAppInstallations() after blocked delete = %+v, %v, want still 1 row", installations, err)
	}
}

func TestGitHubAppInstallationRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/github-app/installations"},
		{http.MethodDelete, "/api/v1/github-app/installations/1"},
	})
}
