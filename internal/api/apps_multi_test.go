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

// TestHandleDeploySpec_Success_CreatesTwoServicesUnderOneApp covers this
// handler's own HTTP-layer contract: request decoding, fetching source
// once, forwarding a correctly-built deploy.MultiRequest to the
// builder, and turning its outcomes into the response shape. Actual
// store persistence (SaveDesiredService/SaveApp/UpdateServiceApp) is
// internal/deploy.Pipeline.DeploySpec's own job, already covered end to
// end by internal/deploy/multi_test.go against a real *store.DB; this
// package's own tests use a fake Builder for the same reason every
// other builds.go/apps_multi.go test in this package already does
// (builds_test.go's own fakeBuilder), so this test does not re-assert
// store state a fake builder never actually writes.
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
		t.Errorf("fetch call = %+v, want repoURL/ref from the request's repo_url/ref", call)
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

// TestHandleDeploySpec_ImageServiceAccepted proves a service declaring
// build.type: image passes through to internal/deploy.Pipeline.DeploySpec
// unchanged, alongside a normal dockerfile service in the same fan-out:
// req.Services is a full spec.Service map already (see deploySpecRequest's
// own doc comment), so this handler needs no per-field reconstruction the
// way handleTriggerBuild's narrower triggerBuildBuildInput does.
func TestHandleDeploySpec_ImageServiceAccepted(t *testing.T) {
	builder := &fakeBuilder{tag: "img:sha"}
	fetch := newFakeFetch("/tmp/checkout", nil)
	rt, db := newTestRouterWithBuilder(t, builder, fetch)
	cookie := loginTestSession(t, rt, db)

	body := `{
		"repo_url": "https://example.com/org/app.git",
		"ref": "main",
		"services": {
			"web": {"build": {"type": "dockerfile", "path": "./web/Dockerfile"}, "port": 3000},
			"cache": {"build": {"type": "image", "image": "redis:7-alpine"}, "port": 6379}
		}
	}`

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/deploy-spec", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	cacheSvc, ok := builder.lastMultiReq.Services["cache"]
	if !ok {
		t.Fatal("Services has no \"cache\" key")
	}
	if cacheSvc.Build.Type != "image" || cacheSvc.Build.Image != "redis:7-alpine" {
		t.Errorf("cache service Build = %+v, want Type=image Image=redis:7-alpine", cacheSvc.Build)
	}
}

// TestHandleDeploySpec_PartialFailure_StillReturns2xxWithPerServiceErrors
// proves a partial fan-out failure is reported per-service, not as a
// whole-request failure: the HTTP request itself succeeded at fanning
// out, matching deploy.Pipeline.DeploySpec's own "one broken resource
// must not block the others" contract.
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

// TestHandleDeploySpec_SingleServiceDeployStillWorks proves stage 2's
// fan-out endpoint existing alongside the pre-existing single-service
// paths (POST /api/v1/apps, POST /api/v1/apps/{name}/builds) doesn't
// change how either of those behaves: seeding and reading a
// single-service app through the ordinary path is unaffected by
// deploy-spec.go's own existence.
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
