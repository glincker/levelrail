package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_GetOAuthSettings(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]OAuthProviderSettingsResource{{Provider: "google", Enabled: true, HasClientSecret: true}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.GetOAuthSettings(context.Background())
	if err != nil {
		t.Fatalf("GetOAuthSettings() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/settings/oauth" {
		t.Errorf("method/path = %s %s, want GET /api/v1/settings/oauth", gotMethod, gotPath)
	}
	if len(got) != 1 || got[0].Provider != "google" {
		t.Errorf("GetOAuthSettings() = %+v, want one google entry", got)
	}
}

func TestClient_GetIngressSettings(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(IngressSettingsResource{PrimaryDomain: "example.com", ACMEEnabled: true, ACMEEmail: "ops@example.com"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.GetIngressSettings(context.Background())
	if err != nil {
		t.Fatalf("GetIngressSettings() error = %v", err)
	}
	if gotPath != "/api/v1/settings/ingress" {
		t.Errorf("path = %s, want /api/v1/settings/ingress", gotPath)
	}
	if got.PrimaryDomain != "example.com" || !got.ACMEEnabled {
		t.Errorf("GetIngressSettings() = %+v, want PrimaryDomain=example.com ACMEEnabled=true", got)
	}
}

func TestClient_UpdateIngressSettings(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody IngressSettingsResource
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.UpdateIngressSettings(context.Background(), IngressSettingsResource{PrimaryDomain: "example.com"})
	if err != nil {
		t.Fatalf("UpdateIngressSettings() error = %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/ingress" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/ingress", gotMethod, gotPath)
	}
	if got.PrimaryDomain != "example.com" {
		t.Errorf("UpdateIngressSettings() = %+v, want PrimaryDomain=example.com", got)
	}
}

func TestClient_SetAppStorage(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody SetAppStorageRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AppStorageResource{AppName: "web", StorageTargetID: gotBody.StorageTargetID})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.SetAppStorage(context.Background(), "web", "target-1")
	if err != nil {
		t.Fatalf("SetAppStorage() error = %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/storage" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/apps/web/storage", gotMethod, gotPath)
	}
	if gotBody.StorageTargetID != "target-1" || got.StorageTargetID != "target-1" {
		t.Errorf("SetAppStorage() = %+v, body = %+v, want StorageTargetID=target-1", got, gotBody)
	}
}

func TestClient_ClearAppStorage(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	if err := client.ClearAppStorage(context.Background(), "web"); err != nil {
		t.Fatalf("ClearAppStorage() error = %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/apps/web/storage" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/apps/web/storage", gotMethod, gotPath)
	}
}

func TestClient_ListGitHubAppRepos(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GitHubAppRepoListResource{
			Repos: []GitHubAppRepoResource{{FullName: "acme/widgets", DefaultBranch: "main", AccountType: "organization"}},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.ListGitHubAppRepos(context.Background())
	if err != nil {
		t.Fatalf("ListGitHubAppRepos() error = %v", err)
	}
	if gotPath != "/api/v1/github-app/repos" {
		t.Errorf("path = %s, want /api/v1/github-app/repos", gotPath)
	}
	if len(got.Repos) != 1 || got.Repos[0].FullName != "acme/widgets" {
		t.Errorf("ListGitHubAppRepos() = %+v, want one acme/widgets entry", got)
	}
}

func TestClient_ListGitHubAppInstallations(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GitHubAppInstallationListResource{
			Installations: []GitHubAppInstallationResource{
				{ID: 1, InstallationID: 42, AccountLogin: "acme", AccountType: "organization", ConnectedAt: "2026-01-01T00:00:00Z"},
			},
			AddOrgURL: "https://github.com/apps/levelrail/installations/new",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.ListGitHubAppInstallations(context.Background())
	if err != nil {
		t.Fatalf("ListGitHubAppInstallations() error = %v", err)
	}
	if gotPath != "/api/v1/github-app/installations" {
		t.Errorf("path = %s, want /api/v1/github-app/installations", gotPath)
	}
	if len(got.Installations) != 1 || got.Installations[0].AccountLogin != "acme" || got.AddOrgURL == "" {
		t.Errorf("ListGitHubAppInstallations() = %+v, want one acme entry with an AddOrgURL", got)
	}
}

func TestClient_DeleteGitHubAppInstallation(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	if err := client.DeleteGitHubAppInstallation(context.Background(), 7); err != nil {
		t.Fatalf("DeleteGitHubAppInstallation() error = %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/github-app/installations/7" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/github-app/installations/7", gotMethod, gotPath)
	}
}

func TestClient_ListGitHubAppBranches(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]GitAppBranchResource{{Name: "main", CommitSHA: "abc123"}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.ListGitHubAppBranches(context.Background(), "acme", "widgets")
	if err != nil {
		t.Fatalf("ListGitHubAppBranches() error = %v", err)
	}
	if gotPath != "/api/v1/github-app/repos/acme/widgets/branches" {
		t.Errorf("path = %s, want /api/v1/github-app/repos/acme/widgets/branches", gotPath)
	}
	if len(got) != 1 || got[0].Name != "main" {
		t.Errorf("ListGitHubAppBranches() = %+v, want one main entry", got)
	}
}

func TestClient_UseGitHubRepoAsSource(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody UseRepoAsSourceRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(UseGitHubRepoAsSourceResponse{
			GitSourceResource: GitSourceResource{ServiceName: gotBody.AppName, RepoURL: "https://github.com/acme/widgets.git"},
			WebhookRegistered: true,
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.UseGitHubRepoAsSource(context.Background(), "acme", "widgets", UseRepoAsSourceRequest{AppName: "web"})
	if err != nil {
		t.Fatalf("UseGitHubRepoAsSource() error = %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/github-app/repos/acme/widgets/use-as-source" {
		t.Errorf("method/path = %s %s, want POST /api/v1/github-app/repos/acme/widgets/use-as-source", gotMethod, gotPath)
	}
	if !got.WebhookRegistered || got.ServiceName != "web" {
		t.Errorf("UseGitHubRepoAsSource() = %+v, want WebhookRegistered=true ServiceName=web", got)
	}
}

func TestClient_ListStaticSites(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]StaticSiteResource{{Name: "marketing", Domains: []string{"example.com"}}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.ListStaticSites(context.Background())
	if err != nil {
		t.Fatalf("ListStaticSites() error = %v", err)
	}
	if gotPath != "/api/v1/static-sites" {
		t.Errorf("path = %s, want /api/v1/static-sites", gotPath)
	}
	if len(got) != 1 || got[0].Name != "marketing" {
		t.Errorf("ListStaticSites() = %+v, want one marketing entry", got)
	}
}
