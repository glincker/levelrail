package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/deploy"
)

func multiDeployBody() string {
	return `{
		"repo_url": "https://example.com/org/app.git",
		"ref": "main",
		"services": {
			"web": {"build": {"type": "dockerfile", "path": "./web/Dockerfile"}, "port": 3000},
			"worker": {"build": {"type": "dockerfile", "path": "./worker/Dockerfile"}, "port": 4000}
		}
	}`
}

func TestHandleDeploySpec_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/myapp/deploy-spec", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleDeploySpec_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithBuilder
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/deploy-spec", multiDeployBody()))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

// TestHandleDeploySpec_Success_CreatesTwoServicesUnderOneApp covers the
// HTTP-layer contract with a fake Builder; store persistence is covered
// separately by internal/deploy/multi_test.go against a real *store.DB.
func TestHandleDeploySpec_Success_CreatesTwoServicesUnderOneApp(t *testing.T) {
	builder := &fakeBuilder{tag: "img:sha"}
	fetch := newFakeFetch("/tmp/checkout", nil)
	rt, db := newTestRouterWithBuilder(t, builder, fetch)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/deploy-spec", multiDeployBody()))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp deploySpecResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	assertDeploySpecAllSucceeded(t, resp)

	if builder.multiCalls != 1 {
		t.Fatalf("builder.multiCalls = %d, want 1", builder.multiCalls)
	}
	assertMultiRequestFromBody(t, builder.lastMultiReq)

	call := fetch.awaitCall(t)
	if call.repoURL != "https://example.com/org/app.git" || call.ref != "main" {
		t.Errorf("fetch call = %+v, want repo_url/ref from the request", call)
	}
}

func assertDeploySpecAllSucceeded(t *testing.T, resp deploySpecResponse) {
	t.Helper()
	if !resp.AllSucceeded {
		t.Errorf("AllSucceeded = false, want true; services = %+v", resp.Services)
	}
	if len(resp.Services) != 2 {
		t.Fatalf("Services = %+v, want 2", resp.Services)
	}
	for _, s := range resp.Services {
		if s.Error != "" {
			t.Errorf("service %+v has an error, want none", s)
		}
		if s.Image == "" {
			t.Errorf("service %+v has no image", s)
		}
	}
}

func assertMultiRequestFromBody(t *testing.T, req deploy.MultiRequest) {
	t.Helper()
	if req.AppName != "myapp" {
		t.Errorf("AppName = %q, want %q", req.AppName, "myapp")
	}
	if req.ImageRepoBase != "myapp" {
		t.Errorf("ImageRepoBase = %q, want %q (defaulted from the path segment)", req.ImageRepoBase, "myapp")
	}
	if req.CommitSHA != "main" {
		t.Errorf("CommitSHA = %q, want %q", req.CommitSHA, "main")
	}
	if req.SourceDir != "/tmp/checkout" {
		t.Errorf("SourceDir = %q, want %q (the fetched checkout dir)", req.SourceDir, "/tmp/checkout")
	}
	if len(req.Services) != 2 {
		t.Errorf("Services = %+v, want 2 keys (web, worker)", req.Services)
	}
}

// TestHandleDeploySpec_PartialFailure_StillReturns2xxWithPerServiceErrors
// proves a partial fan-out failure is reported per-service, not as a
// whole-request failure.
func TestHandleDeploySpec_PartialFailure_StillReturns2xxWithPerServiceErrors(t *testing.T) {
	builder := &fakeBuilder{
		multiOutcomes: []deploy.ServiceOutcome{
			{ServiceKey: "web", ServiceName: "myapp-web", Image: "img:sha"},
			{ServiceKey: "worker", ServiceName: "myapp-worker", Err: errors.New("worker build failed")},
		},
	}
	rt, db := newTestRouterWithBuilder(t, builder, newFakeFetch("/tmp/checkout", nil))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/deploy-spec", multiDeployBody()))
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusMultiStatus, rec.Body.String())
	}

	var resp deploySpecResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AllSucceeded {
		t.Error("AllSucceeded = true, want false")
	}

	var webResult, workerResult deploySpecServiceResult
	for _, s := range resp.Services {
		switch s.ServiceKey {
		case "web":
			webResult = s
		case "worker":
			workerResult = s
		}
	}
	if webResult.Error != "" {
		t.Errorf("web result = %+v, want no error", webResult)
	}
	if workerResult.Error == "" {
		t.Error("worker result has no error, want the simulated failure surfaced")
	}
}

func TestHandleDeploySpec_MissingRepoURL_Rejected(t *testing.T) {
	builder := &fakeBuilder{tag: "img:sha"}
	rt, db := newTestRouterWithBuilder(t, builder, newFakeFetch("/tmp/checkout", nil))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/deploy-spec", `{"ref":"main","services":{"web":{"build":{"type":"dockerfile"}}}}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleDeploySpec_NoServices_Rejected(t *testing.T) {
	builder := &fakeBuilder{tag: "img:sha"}
	rt, db := newTestRouterWithBuilder(t, builder, newFakeFetch("/tmp/checkout", nil))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/deploy-spec", `{"repo_url":"https://example.com/x.git","ref":"main","services":{}}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestHandleDeploySpec_SingleServiceDeployStillWorks proves the fan-out
// endpoint existing doesn't change single-service app behavior.
func TestHandleDeploySpec_SingleServiceDeployStillWorks(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", `{"name":"classic","image":"levelrail/classic:1","port":3000}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	svc, err := db.GetDesiredService(context.Background(), "classic")
	if err != nil {
		t.Fatalf("GetDesiredService(classic): %v", err)
	}
	if svc.Image != "levelrail/classic:1" {
		t.Errorf("Image = %q, want %q", svc.Image, "levelrail/classic:1")
	}
	if len(svc.Domains) != 0 {
		t.Errorf("Domains = %v, want empty (no fan-out fields leaked into a single-service create)", svc.Domains)
	}
}
