package githubapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient starts an httptest.Server driven by handler and returns
// a Client pointed at it. Every test in this file exercises real
// request construction (method, path, headers, body decoding) against
// this fake server; none of it reaches the real api.github.com.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Client{HTTP: srv.Client(), BaseURL: srv.URL}
}

func TestExchangeManifestCode_RequestAndResponseShape(t *testing.T) {
	var gotPath, gotMethod, gotAuth, gotAPIVersion, gotAccept string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAPIVersion = r.Header.Get("X-GitHub-Api-Version")
		gotAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusCreated)
		//nolint:gosec // fake fixture value for a test-only httptest.Server response, not a real credential
		_ = json.NewEncoder(w).Encode(manifestConversionResponse{
			ID:            42,
			ClientID:      "Iv1.abc123",
			ClientSecret:  "supersecret",
			WebhookSecret: "whsecret",
			PEM:           "-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----\n",
			Slug:          "my-app",
			HTMLURL:       "https://github.com/apps/my-app",
		})
	})

	creds, err := c.ExchangeManifestCode(context.Background(), "", "the-code")
	if err != nil {
		t.Fatalf("ExchangeManifestCode() error = %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/app-manifests/the-code/conversions" {
		t.Errorf("path = %q, want /app-manifests/the-code/conversions", gotPath)
	}
	if gotAuth != "" {
		t.Errorf("Authorization header = %q, want empty (this endpoint is documented as not requiring one)", gotAuth)
	}
	if gotAPIVersion != apiVersion {
		t.Errorf("X-GitHub-Api-Version = %q, want %q", gotAPIVersion, apiVersion)
	}
	if gotAccept != "application/vnd.github+json" {
		t.Errorf("Accept = %q, want application/vnd.github+json", gotAccept)
	}

	if creds.AppID != 42 || creds.ClientID != "Iv1.abc123" || creds.ClientSecret != "supersecret" ||
		creds.WebhookSecret != "whsecret" || creds.Slug != "my-app" {
		t.Errorf("ExchangeManifestCode() = %+v, fields did not decode as expected", creds)
	}
}

func TestExchangeManifestCode_CodeIsURLEscaped(t *testing.T) {
	// GitHub's real manifest codes are opaque alphanumeric tokens, never
	// containing "/" or a space, but this proves the code segment is
	// escaped on the wire (r.URL.EscapedPath(), the literal bytes sent)
	// rather than concatenated raw, regardless: a "/" in code must not
	// be interpreted as an extra path segment. r.URL.Path (used by every
	// other test in this file) is deliberately not asserted on here,
	// since net/http always reports that field already-decoded, which
	// would make this specific check pass even for an actual escaping
	// bug.
	var gotEscapedPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotEscapedPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusCreated)
		//nolint:gosec // zero-value fixture, no credential material at all
		_ = json.NewEncoder(w).Encode(manifestConversionResponse{})
	})

	_, err := c.ExchangeManifestCode(context.Background(), "", "a/b c")
	if err != nil {
		t.Fatalf("ExchangeManifestCode() error = %v", err)
	}
	if !strings.Contains(gotEscapedPath, "%2F") || !strings.Contains(gotEscapedPath, "%20") {
		t.Errorf("escaped path = %q, want the code segment's \"/\" and \" \" percent-encoded", gotEscapedPath)
	}
}

func TestExchangeManifestCode_ErrorResponse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	})

	_, err := c.ExchangeManifestCode(context.Background(), "", "bad-code")
	if err == nil {
		t.Fatal("ExchangeManifestCode() error = nil, want a wrapped apiError")
	}
	var apiErr *apiError
	if !asAPIError(err, &apiErr) {
		t.Fatalf("ExchangeManifestCode() error = %v (%T), want *apiError", err, err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("apiError.StatusCode = %d, want 404", apiErr.StatusCode)
	}
}

func asAPIError(err error, target **apiError) bool {
	if e, ok := err.(*apiError); ok {
		*target = e
		return true
	}
	return false
}

func TestGetInstallation_UsesBearerAppJWT(t *testing.T) {
	var gotPath, gotAuth string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(installationResponse{
			ID:    99,
			AppID: 42,
			Account: struct {
				Login string `json:"login"`
			}{Login: "octocat"},
		})
	})

	info, err := c.GetInstallation(context.Background(), "", "the-app-jwt", 99)
	if err != nil {
		t.Fatalf("GetInstallation() error = %v", err)
	}
	if gotPath != "/app/installations/99" {
		t.Errorf("path = %q, want /app/installations/99", gotPath)
	}
	if gotAuth != "Bearer the-app-jwt" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer the-app-jwt")
	}
	if info.ID != 99 || info.AppID != 42 || info.AccountLogin != "octocat" {
		t.Errorf("GetInstallation() = %+v, unexpected", info)
	}
}

func TestMintInstallationToken_ParsesExpiry(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/app/installations/7/access_tokens" {
			t.Errorf("path = %q, want /app/installations/7/access_tokens", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(installationTokenResponse{
			Token:     "ghs_abc123",
			ExpiresAt: "2026-08-14T13:00:00Z",
		})
	})

	tok, err := c.MintInstallationToken(context.Background(), "", "the-app-jwt", 7)
	if err != nil {
		t.Fatalf("MintInstallationToken() error = %v", err)
	}
	if tok.Token != "ghs_abc123" {
		t.Errorf("Token = %q, want ghs_abc123", tok.Token)
	}
	if tok.ExpiresAt.Year() != 2026 {
		t.Errorf("ExpiresAt = %v, did not parse as expected", tok.ExpiresAt)
	}
}

func TestListInstallationRepos_Paginates(t *testing.T) {
	// 250 repos total: 100 + 100 + 50, so this must make exactly 3
	// requests and stop once a short page comes back.
	const total = 250
	var requestCount int
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		page := r.URL.Query().Get("page")
		var pageNum int
		_, _ = fmt.Sscanf(page, "%d", &pageNum)

		start := (pageNum - 1) * listPerPage
		end := start + listPerPage
		if end > total {
			end = total
		}
		var repos []repoResponse
		for i := start; i < end; i++ {
			repos = append(repos, repoResponse{FullName: fmt.Sprintf("owner/repo-%d", i)})
		}
		_ = json.NewEncoder(w).Encode(listRepositoriesResponse{TotalCount: total, Repositories: repos})
	})

	repos, err := c.ListInstallationRepos(context.Background(), "", "tok")
	if err != nil {
		t.Fatalf("ListInstallationRepos() error = %v", err)
	}
	if len(repos) != total {
		t.Errorf("ListInstallationRepos() returned %d repos, want %d", len(repos), total)
	}
	if requestCount != 3 {
		t.Errorf("made %d requests, want 3 (100+100+50)", requestCount)
	}
}

func TestListBranches_UsesInstallationToken(t *testing.T) {
	var gotPath, gotAuth string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode([]branchResponse{
			{Name: "main", Commit: struct {
				SHA string `json:"sha"`
			}{SHA: "abc123"}},
		})
	})

	branches, err := c.ListBranches(context.Background(), "", "install-token", "acme", "widgets")
	if err != nil {
		t.Fatalf("ListBranches() error = %v", err)
	}
	if gotPath != "/repos/acme/widgets/branches" {
		t.Errorf("path = %q, want /repos/acme/widgets/branches", gotPath)
	}
	if gotAuth != "Bearer install-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer install-token")
	}
	if len(branches) != 1 || branches[0].Name != "main" || branches[0].CommitSHA != "abc123" {
		t.Errorf("ListBranches() = %+v, unexpected", branches)
	}
}

func TestGetRepo_UsesInstallationToken(t *testing.T) {
	var gotPath, gotAuth string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(repoResponse{
			FullName: "acme/widgets", Name: "widgets", DefaultBranch: "main",
		})
	})

	repo, err := c.GetRepo(context.Background(), "", "install-token", "acme", "widgets")
	if err != nil {
		t.Fatalf("GetRepo() error = %v", err)
	}
	if gotPath != "/repos/acme/widgets" {
		t.Errorf("path = %q, want /repos/acme/widgets", gotPath)
	}
	if gotAuth != "Bearer install-token" {
		t.Errorf("Authorization = %q, want Bearer install-token", gotAuth)
	}
	if repo.FullName != "acme/widgets" || repo.DefaultBranch != "main" {
		t.Errorf("GetRepo() = %+v, unexpected", repo)
	}
}

func TestCreateRepoWebhook_SendsSecretAndEvents(t *testing.T) {
	var gotPath, gotMethod, gotAuth string
	var gotBody createRepoHookRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	})

	err := c.CreateRepoWebhook(context.Background(), "", "install-token", "acme", "widgets", "https://deploy.example.com/api/v1/webhooks/github/web", "whsecret")
	if err != nil {
		t.Fatalf("CreateRepoWebhook() error = %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/repos/acme/widgets/hooks" {
		t.Errorf("path = %q, want /repos/acme/widgets/hooks", gotPath)
	}
	if gotAuth != "Bearer install-token" {
		t.Errorf("Authorization = %q, want Bearer install-token", gotAuth)
	}
	if len(gotBody.Events) != 1 || gotBody.Events[0] != "push" {
		t.Errorf("events = %v, want [push]", gotBody.Events)
	}
	if gotBody.Config.Secret != "whsecret" {
		t.Errorf("config.secret = %q, want whsecret", gotBody.Config.Secret)
	}
	if gotBody.Config.URL != "https://deploy.example.com/api/v1/webhooks/github/web" {
		t.Errorf("config.url = %q, unexpected", gotBody.Config.URL)
	}
	if !gotBody.Active {
		t.Error("active = false, want true")
	}
}

func TestCreateRepoWebhook_PermissionDenied(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
	})

	err := c.CreateRepoWebhook(context.Background(), "", "install-token", "acme", "widgets", "https://deploy.example.com/hook", "sec")
	if err == nil {
		t.Fatal("CreateRepoWebhook() error = nil, want a 403 error")
	}
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("errors.Is(%v, ErrPermissionDenied) = false, want true", err)
	}
}

func TestCreateRepoWebhook_ErrorResponse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})

	err := c.CreateRepoWebhook(context.Background(), "", "install-token", "acme", "widgets", "https://deploy.example.com/hook", "sec")
	if err == nil {
		t.Fatal("CreateRepoWebhook() error = nil, want an error")
	}
	if errors.Is(err, ErrPermissionDenied) {
		t.Error("errors.Is(err, ErrPermissionDenied) = true, want false for a 502")
	}
}
