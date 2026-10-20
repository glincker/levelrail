package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
)

// newTestRouterWithDetect builds a Router with detect overridden
// directly on the unexported field, the same pattern
// newTestRouterWithListBranches already establishes for listBranches.
func newTestRouterWithDetect(t *testing.T, fn detectFunc) (*Router, *store.DB) {
	t.Helper()
	rt, db := newTestRouter(t)
	rt.detect = fn
	return rt, db
}

func TestHandleDetectFramework_Success(t *testing.T) {
	var gotReq build.DetectRequest
	rt, db := newTestRouterWithDetect(t, func(_ context.Context, req build.DetectRequest) (*build.DetectResult, error) {
		gotReq = req
		return &build.DetectResult{Provider: "node", FrameworkName: "Node.js"}, nil
	})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/build/detect",
		`{"repo_url":"https://example.com/x.git","ref":"main"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotReq.RepoURL != "https://example.com/x.git" || gotReq.Ref != "main" {
		t.Errorf("detect called with %+v, want repo_url/ref threaded through", gotReq)
	}

	var resp detectFrameworkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Detected || resp.Provider != "node" || resp.FrameworkName != "Node.js" {
		t.Errorf("resp = %+v, want detected node/Node.js", resp)
	}
}

func TestHandleDetectFramework_NothingDetected(t *testing.T) {
	rt, db := newTestRouterWithDetect(t, func(context.Context, build.DetectRequest) (*build.DetectResult, error) {
		return &build.DetectResult{}, nil
	})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/build/detect",
		`{"repo_url":"https://example.com/x.git"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp detectFrameworkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Detected || resp.FrameworkName != "" {
		t.Errorf("resp = %+v, want a graceful nothing-detected response", resp)
	}
}

// A clone failure (unreachable repo, timeout, oversized checkout) is not
// surfaced as an HTTP error: the wizard's contract is "fall back to
// manual build type selection," matching handleDetectFramework's own
// doc comment, not a scary error banner for what is very often just an
// unsupported or unreachable repo.
func TestHandleDetectFramework_DetectFailure_RespondsGracefully(t *testing.T) {
	rt, db := newTestRouterWithDetect(t, func(context.Context, build.DetectRequest) (*build.DetectResult, error) {
		return nil, errors.New("build: detect: clone timed out")
	})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/build/detect",
		`{"repo_url":"https://example.com/x.git"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp detectFrameworkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Detected {
		t.Errorf("resp = %+v, want detected: false on a clone failure", resp)
	}
}

func TestHandleDetectFramework_MissingRepoURL(t *testing.T) {
	rt, db := newTestRouterWithDetect(t, func(context.Context, build.DetectRequest) (*build.DetectResult, error) {
		t.Fatal("detect should not be called without repo_url")
		return nil, nil
	})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/build/detect", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
