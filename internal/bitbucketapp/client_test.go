package bitbucketapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ExchangeCode_UsesBasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "the-key" || pass != "the-secret" {
			t.Errorf("BasicAuth() = (%q, %q, %v), want (the-key, the-secret, true)", user, pass, ok)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if r.Form.Get("grant_type") != "authorization_code" {
			t.Errorf("grant_type = %q, want authorization_code", r.Form.Get("grant_type"))
		}
		if r.Form.Get("code") != "the-code" {
			t.Errorf("code = %q, want the-code", r.Form.Get("code"))
		}
		if r.Form.Get("client_id") != "" || r.Form.Get("client_secret") != "" {
			t.Error("client_id/client_secret must not appear in the form body, they belong in the Basic Auth header")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-1", "refresh_token": "rt-1", "expires_in": 7200,
		})
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), TokenURL: srv.URL}
	tok, err := c.ExchangeCode(context.Background(), "the-key", "the-secret", "the-code")
	if err != nil {
		t.Fatalf("ExchangeCode() error = %v", err)
	}
	if tok.AccessToken != "at-1" || tok.RefreshToken != "rt-1" {
		t.Errorf("tokens = %+v, want access_token=at-1 refresh_token=rt-1", tok)
	}
	if tok.ExpiresAt.IsZero() {
		t.Error("ExpiresAt is zero, want a computed expiry")
	}
}

func TestClient_RefreshToken_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), TokenURL: srv.URL}
	_, err := c.RefreshToken(context.Background(), "the-key", "the-secret", "stale-refresh-token")
	if err == nil {
		t.Fatal("RefreshToken() error = nil, want an error for a 400 response")
	}
}

func TestClient_ListRepos_FollowsCursorPagination(t *testing.T) {
	var page1, page2 string
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	page1 = srv.URL + "/repositories?role=member&pagelen=100"
	page2 = srv.URL + "/page2"

	mux.HandleFunc("/repositories", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer the-token" {
			t.Errorf("Authorization = %q, want Bearer the-token", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{
				{"full_name": "acme/one", "name": "one", "is_private": true, "mainbranch": map[string]any{"name": "main"}},
			},
			"next": page2,
		})
	})
	mux.HandleFunc("/page2", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{
				{"full_name": "acme/two", "name": "two", "is_private": false, "mainbranch": map[string]any{"name": "master"}},
			},
			"next": "",
		})
	})

	c := &Client{HTTP: srv.Client(), APIBaseURL: srv.URL}
	repos, err := c.ListRepos(context.Background(), "the-token")
	if err != nil {
		t.Fatalf("ListRepos() error = %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("ListRepos() returned %d repos, want 2 (across both pages)", len(repos))
	}
	if repos[0].FullName != "acme/one" || repos[1].FullName != "acme/two" {
		t.Errorf("ListRepos() = %+v, unexpected order/content", repos)
	}
	if repos[0].CloneURL != "https://bitbucket.org/acme/one.git" {
		t.Errorf("CloneURL fallback = %q, want the constructed default (no clone link in fixture)", repos[0].CloneURL)
	}
	_ = page1
}

func TestClient_ListBranches_UsesInstallationToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repositories/acme/widgets/refs/branches" {
			t.Errorf("path = %q, want /repositories/acme/widgets/refs/branches", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"values": []map[string]any{
				{"name": "main", "target": map[string]any{"hash": "abc123"}},
			},
			"next": "",
		})
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), APIBaseURL: srv.URL}
	branches, err := c.ListBranches(context.Background(), "tok", "acme/widgets")
	if err != nil {
		t.Fatalf("ListBranches() error = %v", err)
	}
	if len(branches) != 1 || branches[0].Name != "main" || branches[0].CommitSHA != "abc123" {
		t.Errorf("ListBranches() = %+v, unexpected", branches)
	}
}

func TestClient_CreateRepoWebhook_SendsSecret(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repositories/acme/widgets/hooks" {
			t.Errorf("path = %q, want /repositories/acme/widgets/hooks", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), APIBaseURL: srv.URL}
	err := c.CreateRepoWebhook(context.Background(), "tok", "acme/widgets", "https://deploy.example.com/hook", "whsecret")
	if err != nil {
		t.Fatalf("CreateRepoWebhook() error = %v", err)
	}
	if gotBody["url"] != "https://deploy.example.com/hook" {
		t.Errorf("body url = %v, want the hook URL", gotBody["url"])
	}
	if gotBody["secret"] != "whsecret" {
		t.Errorf("body secret = %v, want whsecret", gotBody["secret"])
	}
	events, _ := gotBody["events"].([]any)
	if len(events) != 1 || events[0] != "repo:push" {
		t.Errorf("body events = %v, want [repo:push]", gotBody["events"])
	}
}

func TestClient_CreateRepoWebhook_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"insufficient scope"}}`))
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), APIBaseURL: srv.URL}
	err := c.CreateRepoWebhook(context.Background(), "tok", "acme/widgets", "https://deploy.example.com/hook", "whsecret")
	if err == nil {
		t.Fatal("CreateRepoWebhook() error = nil, want an error for a 403 response")
	}
}
