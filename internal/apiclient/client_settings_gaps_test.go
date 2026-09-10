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

func TestClient_UpdateOAuthProviderSettings(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody UpdateOAuthProviderSettingsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(OAuthProviderSettingsResource{Provider: "github", Enabled: gotBody.Enabled, ClientID: gotBody.ClientID, HasClientSecret: true})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.UpdateOAuthProviderSettings(context.Background(), "github", UpdateOAuthProviderSettingsRequest{Enabled: true, ClientID: "abc", ClientSecret: "shh"}) //nolint:gosec // fake fixture
	if err != nil {
		t.Fatalf("UpdateOAuthProviderSettings() error = %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/oauth/github" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/oauth/github", gotMethod, gotPath)
	}
	if gotBody.ClientID != "abc" || !gotBody.Enabled {
		t.Errorf("request body = %+v, want ClientID=abc Enabled=true", gotBody)
	}
	if !got.HasClientSecret {
		t.Errorf("UpdateOAuthProviderSettings() = %+v, want HasClientSecret=true", got)
	}
}

func TestClient_GetEmailSettings(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(EmailSettingsResource{Backend: "smtp", SMTPHost: "smtp.example", SMTPPasswordSet: true})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.GetEmailSettings(context.Background())
	if err != nil {
		t.Fatalf("GetEmailSettings() error = %v", err)
	}
	if gotPath != "/api/v1/settings/email" {
		t.Errorf("path = %s, want /api/v1/settings/email", gotPath)
	}
	if got.Backend != "smtp" || !got.SMTPPasswordSet {
		t.Errorf("GetEmailSettings() = %+v, want Backend=smtp SMTPPasswordSet=true", got)
	}
}

func TestClient_UpdateEmailSettings(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody EmailSettingsResource
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	req := EmailSettingsResource{Backend: "ses", SESRegion: "us-east-1", SESFrom: "a@b.com", SESAccessKeyID: "AKIA"}
	got, err := client.UpdateEmailSettings(context.Background(), req)
	if err != nil {
		t.Fatalf("UpdateEmailSettings() error = %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/email" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/email", gotMethod, gotPath)
	}
	if got.SESRegion != "us-east-1" {
		t.Errorf("UpdateEmailSettings() = %+v, want SESRegion=us-east-1", got)
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
		_ = json.NewEncoder(w).Encode([]GitHubAppRepoResource{{FullName: "acme/widgets", DefaultBranch: "main"}})
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
	if len(got) != 1 || got[0].FullName != "acme/widgets" {
		t.Errorf("ListGitHubAppRepos() = %+v, want one acme/widgets entry", got)
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

func TestClient_ListGitLabAppProjects(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]GitLabAppProjectResource{{ID: 7, PathWithNamespace: "acme/widgets"}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.ListGitLabAppProjects(context.Background())
	if err != nil {
		t.Fatalf("ListGitLabAppProjects() error = %v", err)
	}
	if gotPath != "/api/v1/gitlab-app/projects" {
		t.Errorf("path = %s, want /api/v1/gitlab-app/projects", gotPath)
	}
	if len(got) != 1 || got[0].ID != 7 {
		t.Errorf("ListGitLabAppProjects() = %+v, want one entry with ID=7", got)
	}
}

func TestClient_ListGitLabAppBranches(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]GitAppBranchResource{{Name: "main"}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.ListGitLabAppBranches(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListGitLabAppBranches() error = %v", err)
	}
	if gotPath != "/api/v1/gitlab-app/projects/7/branches" {
		t.Errorf("path = %s, want /api/v1/gitlab-app/projects/7/branches", gotPath)
	}
	if len(got) != 1 {
		t.Errorf("ListGitLabAppBranches() = %+v, want one entry", got)
	}
}

func TestClient_UseGitLabProjectAsSource(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GitSourceResource{ServiceName: "web"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.UseGitLabProjectAsSource(context.Background(), 7, UseRepoAsSourceRequest{AppName: "web"})
	if err != nil {
		t.Fatalf("UseGitLabProjectAsSource() error = %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/gitlab-app/projects/7/use-as-source" {
		t.Errorf("method/path = %s %s, want POST /api/v1/gitlab-app/projects/7/use-as-source", gotMethod, gotPath)
	}
	if got.ServiceName != "web" {
		t.Errorf("UseGitLabProjectAsSource() = %+v, want ServiceName=web", got)
	}
}

func TestClient_ListBitbucketAppRepos(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]BitbucketAppRepoResource{{FullName: "acme/widgets"}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.ListBitbucketAppRepos(context.Background())
	if err != nil {
		t.Fatalf("ListBitbucketAppRepos() error = %v", err)
	}
	if gotPath != "/api/v1/bitbucket-app/repos" {
		t.Errorf("path = %s, want /api/v1/bitbucket-app/repos", gotPath)
	}
	if len(got) != 1 || got[0].FullName != "acme/widgets" {
		t.Errorf("ListBitbucketAppRepos() = %+v, want one acme/widgets entry", got)
	}
}

func TestClient_ListBitbucketAppBranches(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]GitAppBranchResource{{Name: "main"}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.ListBitbucketAppBranches(context.Background(), "acme", "widgets")
	if err != nil {
		t.Fatalf("ListBitbucketAppBranches() error = %v", err)
	}
	if gotPath != "/api/v1/bitbucket-app/repos/acme/widgets/branches" {
		t.Errorf("path = %s, want /api/v1/bitbucket-app/repos/acme/widgets/branches", gotPath)
	}
	if len(got) != 1 {
		t.Errorf("ListBitbucketAppBranches() = %+v, want one entry", got)
	}
}

func TestClient_UseBitbucketRepoAsSource(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GitSourceResource{ServiceName: "web"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.UseBitbucketRepoAsSource(context.Background(), "acme", "widgets", UseRepoAsSourceRequest{AppName: "web"})
	if err != nil {
		t.Fatalf("UseBitbucketRepoAsSource() error = %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/bitbucket-app/repos/acme/widgets/use-as-source" {
		t.Errorf("method/path = %s %s, want POST /api/v1/bitbucket-app/repos/acme/widgets/use-as-source", gotMethod, gotPath)
	}
	if got.ServiceName != "web" {
		t.Errorf("UseBitbucketRepoAsSource() = %+v, want ServiceName=web", got)
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
