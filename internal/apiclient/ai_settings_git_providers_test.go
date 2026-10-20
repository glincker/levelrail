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
		_ = json.NewEncoder(w).Encode(AIAssistantSettingsResource{Configured: true, Active: &AIModelRef{Provider: "anthropic", Model: "claude-sonnet-5"}})
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
	if !got.Configured || got.Active == nil || got.Active.Model != "claude-sonnet-5" {
		t.Errorf("GetAIAssistantSettings() = %+v, want Configured=true Active.Model=claude-sonnet-5", got)
	}
}

func TestClient_SetAIChoice(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody AIChoiceRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AIAssistantSettingsResource{Configured: true, Active: gotBody.Active})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	got, err := client.SetAIChoice(context.Background(), AIChoiceRequest{Active: &AIModelRef{Provider: "anthropic", Model: "claude-sonnet-5"}})
	if err != nil {
		t.Fatalf("SetAIChoice() error = %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/ai-assistant" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/ai-assistant", gotMethod, gotPath)
	}
	if gotBody.Active == nil || gotBody.Active.Model != "claude-sonnet-5" || !got.Configured {
		t.Errorf("body = %+v, got = %+v, want the active model echoed back", gotBody, got)
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

func TestClient_GitConnections(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.RequestURI())
		_ = json.NewEncoder(w).Encode([]GitProviderConnectionResource{{ID: "gc_1", Provider: "gitlab"}})
	}))
	defer srv.Close()
	client := NewClient(srv.URL, "tok")
	ctx := context.Background()
	if got, err := client.ListGitConnections(ctx, "gitlab"); err != nil || len(got) != 1 || got[0].ID != "gc_1" {
		t.Fatalf("ListGitConnections = %+v, %v", got, err)
	}
	if _, err := client.DisconnectGitConnection(ctx, "gc_1"); err == nil {
		t.Log("decode of a list into an object is allowed to fail; only the request shape matters here")
	}
	want := []string{"GET /api/v1/git-connections?provider=gitlab", "DELETE /api/v1/git-connections/gc_1"}
	for i, w := range want {
		if i >= len(seen) || seen[i] != w {
			t.Errorf("request %d = %v, want %q", i, seen, w)
		}
	}
}
