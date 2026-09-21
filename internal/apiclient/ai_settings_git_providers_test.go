package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_GetAIAssistantSettings(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AIAssistantSettingsResource{Configured: true, Provider: "anthropic", Model: "claude-sonnet-5"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.GetAIAssistantSettings(context.Background())
	if err != nil {
		t.Fatalf("GetAIAssistantSettings() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/settings/ai-assistant" {
		t.Errorf("method/path = %s %s, want GET /api/v1/settings/ai-assistant", gotMethod, gotPath)
	}
	if !got.Configured || got.Model != "claude-sonnet-5" {
		t.Errorf("GetAIAssistantSettings() = %+v, want Configured=true Model=claude-sonnet-5", got)
	}
}

func TestClient_UpdateAIAssistantSettings(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody UpdateAIAssistantSettingsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AIAssistantSettingsResource{Configured: true, Provider: gotBody.Provider, Model: gotBody.Model})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.UpdateAIAssistantSettings(context.Background(), UpdateAIAssistantSettingsRequest{
		Provider: "anthropic", Model: "claude-sonnet-5", APIKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("UpdateAIAssistantSettings() error = %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/ai-assistant" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/ai-assistant", gotMethod, gotPath)
	}
	if gotBody.APIKey != "sk-test" {
		t.Errorf("request body = %+v, want APIKey=sk-test", gotBody)
	}
	if !got.Configured {
		t.Errorf("UpdateAIAssistantSettings() = %+v, want Configured=true", got)
	}
}

func TestClient_DeleteAIAssistantSettings(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AIAssistantSettingsResource{Configured: false})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.DeleteAIAssistantSettings(context.Background())
	if err != nil {
		t.Fatalf("DeleteAIAssistantSettings() error = %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/settings/ai-assistant" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/settings/ai-assistant", gotMethod, gotPath)
	}
	if got.Configured {
		t.Errorf("DeleteAIAssistantSettings() = %+v, want Configured=false", got)
	}
}

func TestClient_ListGitProviders(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]GitProviderResource{
			{Provider: "github", Connected: true, CanListBranches: true, CanRegisterWebhook: true, CanAuthClone: true},
			{Provider: "gitlab", Connected: false},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.ListGitProviders(context.Background())
	if err != nil {
		t.Fatalf("ListGitProviders() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/git-providers" {
		t.Errorf("method/path = %s %s, want GET /api/v1/git-providers", gotMethod, gotPath)
	}
	if len(got) != 2 || got[0].Provider != "github" || !got[0].Connected || got[1].Connected {
		t.Errorf("ListGitProviders() = %+v, want [github connected=true, gitlab connected=false]", got)
	}
}

func TestClient_GetGitHubAppStatus(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GitHubAppStatusResource{Connected: true, Installed: true, AccountLogin: "octocat"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.GetGitHubAppStatus(context.Background())
	if err != nil {
		t.Fatalf("GetGitHubAppStatus() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/github-app" {
		t.Errorf("method/path = %s %s, want GET /api/v1/github-app", gotMethod, gotPath)
	}
	if !got.Connected || got.AccountLogin != "octocat" {
		t.Errorf("GetGitHubAppStatus() = %+v, want Connected=true AccountLogin=octocat", got)
	}
}

func TestClient_DisconnectGitHubApp(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	if err := client.DisconnectGitHubApp(context.Background()); err != nil {
		t.Fatalf("DisconnectGitHubApp() error = %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/github-app" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/github-app", gotMethod, gotPath)
	}
}

func TestClient_GetGitLabAppStatus(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GitLabAppStatusResource{Connected: true, Authorized: true})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.GetGitLabAppStatus(context.Background())
	if err != nil {
		t.Fatalf("GetGitLabAppStatus() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/gitlab-app" {
		t.Errorf("method/path = %s %s, want GET /api/v1/gitlab-app", gotMethod, gotPath)
	}
	if !got.Connected || !got.Authorized {
		t.Errorf("GetGitLabAppStatus() = %+v, want Connected=true Authorized=true", got)
	}
}

func TestClient_DisconnectGitLabApp(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	if err := client.DisconnectGitLabApp(context.Background()); err != nil {
		t.Fatalf("DisconnectGitLabApp() error = %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/gitlab-app" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/gitlab-app", gotMethod, gotPath)
	}
}

func TestClient_GetBitbucketAppStatus(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(BitbucketAppStatusResource{Connected: true, Authorized: true, Key: "abc123"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.GetBitbucketAppStatus(context.Background())
	if err != nil {
		t.Fatalf("GetBitbucketAppStatus() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/bitbucket-app" {
		t.Errorf("method/path = %s %s, want GET /api/v1/bitbucket-app", gotMethod, gotPath)
	}
	if !got.Connected || got.Key != "abc123" {
		t.Errorf("GetBitbucketAppStatus() = %+v, want Connected=true Key=abc123", got)
	}
}

func TestClient_DisconnectBitbucketApp(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	if err := client.DisconnectBitbucketApp(context.Background()); err != nil {
		t.Fatalf("DisconnectBitbucketApp() error = %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/bitbucket-app" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/bitbucket-app", gotMethod, gotPath)
	}
}
