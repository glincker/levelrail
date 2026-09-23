package gitlabapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthorizeURL(t *testing.T) {
	tests := []struct {
		name        string
		instanceURL string
		want        string
	}{
		{
			name:        "gitlab.com",
			instanceURL: "https://gitlab.com",
			want:        "https://gitlab.com/oauth/authorize?client_id=abc&redirect_uri=https%3A%2F%2Fexample.com%2Fcb&response_type=code&scope=api&state=xyz",
		},
		{
			name:        "self-hosted with trailing slash",
			instanceURL: "https://gitlab.internal.example.com/",
			want:        "https://gitlab.internal.example.com/oauth/authorize?client_id=abc&redirect_uri=https%3A%2F%2Fexample.com%2Fcb&response_type=code&scope=api&state=xyz",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AuthorizeURL(tt.instanceURL, "abc", "https://example.com/cb", "xyz")
			if got != tt.want {
				t.Errorf("AuthorizeURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClient_ExchangeCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Errorf("path = %q, want /oauth/token", r.URL.Path)
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
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-1", "refresh_token": "rt-1", "expires_in": 7200, "created_at": 0,
		})
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	tok, err := c.ExchangeCode(context.Background(), srv.URL, "client", "secret", "https://example.com/cb", "the-code")
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
	_, err := c.RefreshToken(context.Background(), srv.URL, "client", "secret", "stale-refresh-token")
	if err == nil {
		t.Fatal("RefreshToken() error = nil, want an error for a 400 response")
	}
}

func TestClient_ListProjects_Paginates(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer at-1" {
			t.Errorf("Authorization header = %q, want Bearer at-1", r.Header.Get("Authorization"))
		}
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		if page == "1" {
			projects := make([]projectResponse, listPerPage)
			for i := range projects {
				projects[i] = projectResponse{ID: int64(i + 1), Name: "p"}
			}
			_ = json.NewEncoder(w).Encode(projects)
			return
		}
		_ = json.NewEncoder(w).Encode([]projectResponse{{ID: 999, Name: "last"}})
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	got, err := c.ListProjects(context.Background(), srv.URL, "at-1")
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(got) != listPerPage+1 {
		t.Errorf("len(got) = %d, want %d", len(got), listPerPage+1)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 pages fetched", calls)
	}
}

func TestClient_ListBranches_Paginates(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v4/projects/7/repository/branches" {
			t.Errorf("path = %q, want /api/v4/projects/7/repository/branches", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer at-1" {
			t.Errorf("Authorization header = %q, want Bearer at-1", r.Header.Get("Authorization"))
		}
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		if page == "1" {
			branches := make([]branchResponse, listPerPage)
			for i := range branches {
				branches[i] = branchResponse{Name: "b"}
				branches[i].Commit.ID = "sha"
			}
			_ = json.NewEncoder(w).Encode(branches)
			return
		}
		last := branchResponse{Name: "main"}
		last.Commit.ID = "abc123"
		_ = json.NewEncoder(w).Encode([]branchResponse{last})
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	got, err := c.ListBranches(context.Background(), srv.URL, "at-1", 7)
	if err != nil {
		t.Fatalf("ListBranches() error = %v", err)
	}
	if len(got) != listPerPage+1 {
		t.Errorf("len(got) = %d, want %d", len(got), listPerPage+1)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 pages fetched", calls)
	}
	last := got[len(got)-1]
	if last.Name != "main" || last.CommitSHA != "abc123" {
		t.Errorf("last branch = %+v, want name=main commit_sha=abc123", last)
	}
}

func TestClient_GetProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42" {
			t.Errorf("path = %q, want /api/v4/projects/42", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(projectResponse{
			ID: 42, Name: "widgets", PathWithNamespace: "acme/widgets",
			HTTPURLToRepo: "https://gitlab.example.com/acme/widgets.git", DefaultBranch: "main",
		})
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	got, err := c.GetProject(context.Background(), srv.URL, "at-1", 42)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if got.ID != 42 || got.PathWithNamespace != "acme/widgets" {
		t.Errorf("GetProject() = %+v, want id=42 path=acme/widgets", got)
	}
}

func TestClient_CreateProjectWebhook(t *testing.T) {
	var gotBody createWebhookRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/api/v4/projects/42/hooks" {
			t.Errorf("path = %q, want /api/v4/projects/42/hooks", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	err := c.CreateProjectWebhook(context.Background(), srv.URL, "at-1", 42, "https://deploy.example.com/api/v1/webhooks/github/myapp", "the-secret")
	if err != nil {
		t.Fatalf("CreateProjectWebhook() error = %v", err)
	}
	if !gotBody.PushEvents {
		t.Error("push_events = false, want true")
	}
	if !gotBody.MergeRequestsEvents {
		t.Error("merge_requests_events = false, want true")
	}
	if gotBody.Token != "the-secret" {
		t.Errorf("token = %q, want the-secret", gotBody.Token)
	}
	if !gotBody.EnableSSLVerification {
		t.Error("enable_ssl_verification = false, want true")
	}
}

func TestClient_CreateProjectWebhook_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	err := c.CreateProjectWebhook(context.Background(), srv.URL, "at-1", 1, "https://example.com/hook", "secret")
	if err == nil {
		t.Fatal("CreateProjectWebhook() error = nil, want an error for a 403 response")
	}
}

func TestEncodedProjectID(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"org/web", "org%2Fweb"},
		{"org/team/web", "org%2Fteam%2Fweb"},
		{"org/we b", "org%2Fwe%20b"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := encodedProjectID(tt.path); got != tt.want {
				t.Errorf("encodedProjectID(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestClient_CreateMergeRequestNote(t *testing.T) {
	var gotBody createNoteRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v4/projects/org%2Fweb/merge_requests/42/notes" {
			t.Errorf("path = %q, want /api/v4/projects/org%%2Fweb/merge_requests/42/notes", r.URL.EscapedPath())
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	err := c.CreateMergeRequestNote(context.Background(), srv.URL, "at-1", "org/web", 42, "preview deployed")
	if err != nil {
		t.Fatalf("CreateMergeRequestNote() error = %v", err)
	}
	if gotBody.Body != "preview deployed" {
		t.Errorf("body = %q, want %q", gotBody.Body, "preview deployed")
	}
}

func TestClient_CreateMergeRequestNote_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	err := c.CreateMergeRequestNote(context.Background(), srv.URL, "at-1", "org/web", 42, "body")
	if err == nil {
		t.Fatal("CreateMergeRequestNote() error = nil, want an error for a 403 response")
	}
}

func TestClient_CreateCommitStatus(t *testing.T) {
	var gotBody createCommitStatusRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v4/projects/org%2Fweb/statuses/sha1" {
			t.Errorf("path = %q, want /api/v4/projects/org%%2Fweb/statuses/sha1", r.URL.EscapedPath())
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	err := c.CreateCommitStatus(context.Background(), srv.URL, "at-1", "org/web", "sha1", CommitStateSuccess, "https://preview.example.com", "Preview deployed", "levelrail/preview")
	if err != nil {
		t.Fatalf("CreateCommitStatus() error = %v", err)
	}
	if gotBody.State != "success" {
		t.Errorf("state = %q, want success", gotBody.State)
	}
	if gotBody.TargetURL != "https://preview.example.com" {
		t.Errorf("target_url = %q, want https://preview.example.com", gotBody.TargetURL)
	}
	if gotBody.Name != "levelrail/preview" {
		t.Errorf("name = %q, want levelrail/preview", gotBody.Name)
	}
}

func TestClient_CreateCommitStatus_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	err := c.CreateCommitStatus(context.Background(), srv.URL, "at-1", "org/web", "sha1", CommitStateFailed, "", "failed", "levelrail/preview")
	if err == nil {
		t.Fatal("CreateCommitStatus() error = nil, want an error for a 404 response")
	}
}
