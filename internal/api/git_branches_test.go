package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// newTestRouterWithListBranches builds a Router with listBranches
// overridden directly on the unexported field, the same "no public
// Option, nothing outside this package needs to fake git I/O" pattern
// newTestRouterWithBuilder already establishes for fetch.
func newTestRouterWithListBranches(t *testing.T, fn listBranchesFunc) (*Router, *store.DB) {
	t.Helper()
	rt, db := newTestRouter(t)
	rt.listBranches = fn
	return rt, db
}

func TestHandleListGitBranches_Success(t *testing.T) {
	calls := 0
	var gotRepoURL string
	rt, db := newTestRouterWithListBranches(t, func(_ context.Context, repoURL string) ([]string, error) {
		calls++
		gotRepoURL = repoURL
		return []string{"main", "develop"}, nil
	})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/git/branches", `{"repo_url":"https://example.com/x.git"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if calls != 1 {
		t.Fatalf("listBranches called %d times, want 1", calls)
	}
	if gotRepoURL != "https://example.com/x.git" {
		t.Errorf("repoURL = %q, want %q", gotRepoURL, "https://example.com/x.git")
	}

	var resp gitBranchesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Branches) != 2 || resp.Branches[0] != "main" || resp.Branches[1] != "develop" {
		t.Errorf("Branches = %v, want [main develop]", resp.Branches)
	}
}

func TestHandleListGitBranches_UnreachableRepo_ReturnsCleanBadRequest(t *testing.T) {
	rt, db := newTestRouterWithListBranches(t, func(_ context.Context, _ string) ([]string, error) {
		return nil, errors.New(`api: list branches: "https://example.com/private.git": authentication required`)
	})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/git/branches", `{"repo_url":"https://example.com/private.git"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "public") || !strings.Contains(body, "repository") {
		t.Errorf("response body = %q, want a clear caller-actionable message", body)
	}
	// The underlying transport error (which can carry hostnames or auth
	// detail) must never leak into the response body, the same
	// non-leaky-error convention handleTriggerBuild's own build-failure
	// path already follows.
	if strings.Contains(body, "authentication required") {
		t.Errorf("response body leaked the underlying transport error: %q", body)
	}
}

func TestHandleListGitBranches_MissingRepoURL(t *testing.T) {
	rt, db := newTestRouterWithListBranches(t, func(_ context.Context, _ string) ([]string, error) {
		t.Fatal("listBranches must not be called when repo_url is missing")
		return nil, nil
	})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/git/branches", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleListGitBranches_InvalidBody(t *testing.T) {
	rt, db := newTestRouterWithListBranches(t, nil)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/git/branches", `not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleListGitBranches_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/git/branches", strings.NewReader(`{"repo_url":"https://example.com/x.git"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
