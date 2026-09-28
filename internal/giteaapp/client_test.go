package giteaapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ExchangeCode_SendsAuthorizationCodeGrant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if r.Form.Get("grant_type") != "authorization_code" {
			t.Errorf("grant_type = %q, want authorization_code", r.Form.Get("grant_type"))
		}
		if r.Form.Get("client_id") != "the-id" || r.Form.Get("client_secret") != "the-secret" {
			t.Errorf("client_id/client_secret = %q/%q, want the-id/the-secret", r.Form.Get("client_id"), r.Form.Get("client_secret"))
		}
		if r.Form.Get("code") != "the-code" {
			t.Errorf("code = %q, want the-code", r.Form.Get("code"))
		}
		if r.URL.Path != "/login/oauth/access_token" {
			t.Errorf("path = %q, want /login/oauth/access_token", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-1", "refresh_token": "rt-1", "expires_in": 3600,
		})
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	tok, err := c.ExchangeCode(context.Background(), srv.URL, "the-id", "the-secret", "https://cp.example.com/api/v1/gitea-app/callback", "the-code")
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

	c := &Client{HTTP: srv.Client()}
	_, err := c.RefreshToken(context.Background(), srv.URL, "the-id", "the-secret", "stale-refresh-token")
	if err == nil {
		t.Fatal("RefreshToken() error = nil, want an error for a 400 response")
	}
}

func TestClient_ListRepos_FollowsPagePagination(t *testing.T) {
	page1 := []map[string]any{
		{"full_name": "acme/one", "name": "one", "private": true, "default_branch": "main", "clone_url": "https://gitea.example.com/acme/one.git", "html_url": "https://gitea.example.com/acme/one"},
	}

	var gotPages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer the-token" {
			t.Errorf("Authorization = %q, want Bearer the-token", got)
		}
		gotPages = append(gotPages, r.URL.Query().Get("page"))
		_ = json.NewEncoder(w).Encode(page1)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	repos, err := c.ListRepos(context.Background(), srv.URL, "the-token")
	if err != nil {
		t.Fatalf("ListRepos() error = %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("ListRepos() returned %d repos, want 1 (page shorter than limit stops pagination)", len(repos))
	}
	if repos[0].FullName != "acme/one" || !repos[0].Private || repos[0].DefaultBranch != "main" {
		t.Errorf("ListRepos() = %+v, unexpected", repos)
	}
	if len(gotPages) != 1 || gotPages[0] != "1" {
		t.Errorf("requested pages = %v, want a single request for page 1", gotPages)
	}
}

func TestClient_GetRepo_BuildsOwnerRepoPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/acme/widgets" {
			t.Errorf("path = %q, want /api/v1/repos/acme/widgets", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"full_name": "acme/widgets", "name": "widgets", "default_branch": "main",
		})
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	repo, err := c.GetRepo(context.Background(), srv.URL, "tok", "acme/widgets")
	if err != nil {
		t.Fatalf("GetRepo() error = %v", err)
	}
	if repo.FullName != "acme/widgets" {
		t.Errorf("GetRepo() = %+v, unexpected", repo)
	}
}

func TestClient_ListBranches_ParsesCommitID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/acme/widgets/branches" {
			t.Errorf("path = %q, want /api/v1/repos/acme/widgets/branches", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"name": "main", "commit": map[string]any{"id": "abc123"}},
		})
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	branches, err := c.ListBranches(context.Background(), srv.URL, "tok", "acme/widgets")
	if err != nil {
		t.Fatalf("ListBranches() error = %v", err)
	}
	if len(branches) != 1 || branches[0].Name != "main" || branches[0].CommitSHA != "abc123" {
		t.Errorf("ListBranches() = %+v, unexpected", branches)
	}
}

func TestClient_CreateRepoWebhook_SendsGiteaShapedConfig(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/acme/widgets/hooks" {
			t.Errorf("path = %q, want /api/v1/repos/acme/widgets/hooks", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	err := c.CreateRepoWebhook(context.Background(), srv.URL, "tok", "acme/widgets", "https://deploy.example.com/hook", "whsecret")
	if err != nil {
		t.Fatalf("CreateRepoWebhook() error = %v", err)
	}
	if gotBody["type"] != "gitea" {
		t.Errorf("body type = %v, want gitea", gotBody["type"])
	}
	config, _ := gotBody["config"].(map[string]any)
	if config["url"] != "https://deploy.example.com/hook" || config["secret"] != "whsecret" || config["content_type"] != "json" {
		t.Errorf("body config = %v, unexpected", config)
	}
	events, _ := gotBody["events"].([]any)
	if len(events) != 2 || events[0] != "push" || events[1] != "pull_request" {
		t.Errorf("body events = %v, want [push pull_request]", gotBody["events"])
	}
}

func TestClient_CreateRepoWebhook_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"insufficient scope"}`))
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	err := c.CreateRepoWebhook(context.Background(), srv.URL, "tok", "acme/widgets", "https://deploy.example.com/hook", "whsecret")
	if err == nil {
		t.Fatal("CreateRepoWebhook() error = nil, want an error for a 403 response")
	}
}

func TestClient_CreateIssueComment(t *testing.T) {
	var gotBody createIssueCommentRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/acme/widgets/issues/42/comments" {
			t.Errorf("path = %q, want /api/v1/repos/acme/widgets/issues/42/comments", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":9}`))
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	id, err := c.CreateIssueComment(context.Background(), srv.URL, "tok", "acme/widgets", 42, "preview deployed")
	if err != nil {
		t.Fatalf("CreateIssueComment() error = %v", err)
	}
	if id != 9 {
		t.Errorf("comment id = %d, want 9", id)
	}
	if gotBody.Body != "preview deployed" {
		t.Errorf("body = %q, want %q", gotBody.Body, "preview deployed")
	}
}

func TestClient_CreateIssueComment_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	_, err := c.CreateIssueComment(context.Background(), srv.URL, "tok", "acme/widgets", 42, "body")
	if err == nil {
		t.Fatal("CreateIssueComment() error = nil, want an error for a 404 response")
	}
}

func TestClient_CreateCommitStatus(t *testing.T) {
	var gotBody createCommitStatusRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/acme/widgets/statuses/sha1" {
			t.Errorf("path = %q, want /api/v1/repos/acme/widgets/statuses/sha1", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	err := c.CreateCommitStatus(context.Background(), srv.URL, "tok", "acme/widgets", "sha1", CommitStatusSuccess, "https://preview.example.com", "Preview deployed", "levelrail/preview")
	if err != nil {
		t.Fatalf("CreateCommitStatus() error = %v", err)
	}
	if gotBody.State != "success" {
		t.Errorf("state = %q, want success", gotBody.State)
	}
	if gotBody.Context != "levelrail/preview" {
		t.Errorf("context = %q, want levelrail/preview", gotBody.Context)
	}
}

func TestClient_CreateCommitStatus_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	err := c.CreateCommitStatus(context.Background(), srv.URL, "tok", "acme/widgets", "sha1", CommitStatusFailure, "", "failed", "levelrail/preview")
	if err == nil {
		t.Fatal("CreateCommitStatus() error = nil, want an error for a 404 response")
	}
}
