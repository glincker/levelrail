package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
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

// TestHandleDeploySpec_ImageBuildTypeAccepted proves build.type: image
// reaches the builder rather than being rejected by this handler's own
// per-service switch, alongside a sibling service that still needs a
// real build: the whole point of supporting it here (rather than only
// through the single-service, no-git POST /apps/{name}/deploys path) is
// that one app.yaml can mix a prebuilt-image service with others that
// don't.
func TestHandleDeploySpec_ImageBuildTypeAccepted(t *testing.T) {
	builder := &fakeBuilder{tag: "img:sha"}
	fetch := newFakeFetch("/tmp/checkout", nil)
	rt, db := newTestRouterWithBuilder(t, builder, fetch)
	cookie := loginTestSession(t, rt, db)

	body := `{
		"repo_url": "https://example.com/org/app.git",
		"ref": "main",
		"services": {
			"web": {"build": {"type": "dockerfile", "path": "./web/Dockerfile"}, "port": 3000},
			"cache-admin": {"build": {"type": "image", "image": "ghcr.io/org/cache-admin:v1"}, "port": 8081}
		}
	}`

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/deploy-spec", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if builder.multiCalls != 1 {
		t.Fatalf("builder.multiCalls = %d, want 1: build.type image must not be rejected before reaching the builder", builder.multiCalls)
	}

	var resp deploySpecResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.AllSucceeded {
		t.Errorf("AllSucceeded = false, want true; services = %+v", resp.Services)
	}
}

// TestHandleDeploySpec_ComposeBuildTypeAccepted proves this handler's
// own per-service switch no longer rejects build.type: compose (it did,
// with a 501, before internal/deploy.Pipeline.DeploySpec grew the
// ability to expand one). Expansion itself (reading and parsing the
// referenced compose file, splicing its own services in) is
// Pipeline.DeploySpec's own job, already covered end to end against a
// real *store.DB by internal/deploy/multi_test.go's own compose tests;
// this test only needs a fake builder (this package's usual pattern) to
// prove the request reaches it at all.
func TestHandleDeploySpec_ComposeBuildTypeAccepted(t *testing.T) {
	builder := &fakeBuilder{tag: "img:sha"}
	fetch := newFakeFetch("/tmp/checkout", nil)
	rt, db := newTestRouterWithBuilder(t, builder, fetch)
	cookie := loginTestSession(t, rt, db)

	body := `{
		"repo_url": "https://example.com/org/app.git",
		"ref": "main",
		"services": {
			"stack": {"build": {"type": "compose", "path": "./docker-compose.yml"}}
		}
	}`

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/deploy-spec", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if builder.multiCalls != 1 {
		t.Fatalf("builder.multiCalls = %d, want 1: build.type compose must not be rejected before reaching the builder", builder.multiCalls)
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

func newTestRouterWithBuilderAndSecrets(t *testing.T, builder Builder, fetch *fakeFetch, setter SecretSetter) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithBuilder(builder), WithSecretSetter(setter))
	if fetch != nil {
		rt.fetch = fetch.fetch
	}
	return rt, db
}

// TestHandleDeploySpec_WithInlineSecret_Success covers an env entry with
// both secret: true and an inline value: the value must reach
// SecretSetter under "<app>-<service key>" (internal/deploy/multi.go's
// own naming convention) before the builder ever runs, so a { required:
// true } secret's own validateEnv check (internal/deploy.Pipeline) sees
// it as already set.
func TestHandleDeploySpec_WithInlineSecret_Success(t *testing.T) {
	builder := &fakeBuilder{tag: "img:sha"}
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithBuilderAndSecrets(t, builder, newFakeFetch("/tmp/checkout", nil), setter)
	cookie := loginTestSession(t, rt, db)

	body := `{
		"repo_url": "https://example.com/org/app.git",
		"ref": "main",
		"services": {
			"web": {"build": {"type": "dockerfile"}, "port": 3000, "env": {"API_KEY": {"secret": true, "required": true, "value": "sk-abc"}}}
		}
	}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/deploy-spec", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sk-abc") {
		t.Errorf("body = %s, must never echo a secret value back", rec.Body.String())
	}
	if setter.calls != 1 || setter.lastService != "myapp-web" || setter.lastKey != "API_KEY" || setter.lastValue != "sk-abc" {
		t.Errorf("setter called with (%q, %q, %q), calls=%d, want (myapp-web, API_KEY, sk-abc), calls=1",
			setter.lastService, setter.lastKey, setter.lastValue, setter.calls)
	}
	if builder.multiCalls != 1 {
		t.Fatalf("builder.multiCalls = %d, want 1", builder.multiCalls)
	}
}

// TestHandleDeploySpec_SecretDeclaredNoValue_BuilderStillRuns covers
// declaring a secret's NAME with no inline value: no SetValueGuarded
// call happens, and the request still reaches the builder (an
// unresolved value is Pipeline.Deploy's own concern via validateEnv, not
// this handler's).
func TestHandleDeploySpec_SecretDeclaredNoValue_BuilderStillRuns(t *testing.T) {
	builder := &fakeBuilder{tag: "img:sha"}
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithBuilderAndSecrets(t, builder, newFakeFetch("/tmp/checkout", nil), setter)
	cookie := loginTestSession(t, rt, db)

	body := `{
		"repo_url": "https://example.com/org/app.git",
		"ref": "main",
		"services": {
			"web": {"build": {"type": "dockerfile"}, "port": 3000, "env": {"API_KEY": {"secret": true, "required": true}}}
		}
	}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/deploy-spec", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if setter.calls != 0 {
		t.Errorf("setter.calls = %d, want 0: no value was given", setter.calls)
	}
	if builder.multiCalls != 1 {
		t.Fatalf("builder.multiCalls = %d, want 1", builder.multiCalls)
	}
}

// TestHandleDeploySpec_Secrets_NotConfigured_Returns501 covers a control
// plane with no master key configured: an inline secret value must fail
// fast, before any git fetch or build work starts.
func TestHandleDeploySpec_Secrets_NotConfigured_Returns501(t *testing.T) {
	builder := &fakeBuilder{tag: "img:sha"}
	rt, db := newTestRouterWithBuilder(t, builder, newFakeFetch("/tmp/checkout", nil)) // no WithSecretSetter
	cookie := loginTestSession(t, rt, db)

	body := `{
		"repo_url": "https://example.com/org/app.git",
		"ref": "main",
		"services": {
			"web": {"build": {"type": "dockerfile"}, "port": 3000, "env": {"API_KEY": {"secret": true, "value": "sk-abc"}}}
		}
	}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/deploy-spec", body))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
	if builder.multiCalls != 0 {
		t.Errorf("builder.multiCalls = %d, want 0: must fail before reaching the builder", builder.multiCalls)
	}
}

// TestHandleDeploySpec_Secrets_Locked_Returns409 mirrors
// TestHandleCreateApp_Secrets_Locked_Returns409 for the fan-out path.
func TestHandleDeploySpec_Secrets_Locked_Returns409(t *testing.T) {
	builder := &fakeBuilder{tag: "img:sha"}
	setter := &fakeSecretSetter{locked: true}
	rt, db := newTestRouterWithBuilderAndSecrets(t, builder, newFakeFetch("/tmp/checkout", nil), setter)
	cookie := loginTestSession(t, rt, db)

	body := `{
		"repo_url": "https://example.com/org/app.git",
		"ref": "main",
		"services": {
			"web": {"build": {"type": "dockerfile"}, "port": 3000, "env": {"API_KEY": {"secret": true, "value": "sk-new"}}}
		}
	}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/deploy-spec", body))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if builder.multiCalls != 0 {
		t.Errorf("builder.multiCalls = %d, want 0", builder.multiCalls)
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

const bindMountDeploySpecBody = `{
	"repo_url": "https://example.com/org/app.git",
	"ref": "main",
	"services": {
		"web": {"build": {"type": "dockerfile", "path": "./Dockerfile"}, "port": 3000, "volumes": [{"hostPath": "/srv/myapp/data", "path": "/data"}]}
	}
}`

// TestHandleDeploySpec_BindMount_PlainDeployToken_Forbidden mirrors
// TestHandleDeployCompose_BindMount_PlainDeployToken_Forbidden
// (apps_compose_test.go): an app.yaml service declaring a bind mount
// needs AbilityRoot on top of this route's own AbilityDeploy gate, same
// as the compose-import path already requires.
func TestHandleDeploySpec_BindMount_PlainDeployToken_Forbidden(t *testing.T) {
	builder := &fakeBuilder{tag: "img:sha"}
	rt, db := newTestRouterWithBuilder(t, builder, newFakeFetch("/tmp/checkout", nil))
	ctx := context.Background()

	const plaintext = "deploy-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_deploy", Name: "deployer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityDeploy}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/myapp/deploy-spec", strings.NewReader(bindMountDeploySpecBody))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain deploy token must not be able to bind-mount a host directory", rec.Code, http.StatusForbidden)
	}
	if builder.multiCalls != 0 {
		t.Errorf("builder.multiCalls = %d, want 0 (rejected before ever reaching the builder)", builder.multiCalls)
	}
}

// TestHandleDeploySpec_BindMount_RootCaller_Succeeds is the positive
// counterpart: a root-ability caller can deploy the exact same
// bind-mounting app.yaml service.
func TestHandleDeploySpec_BindMount_RootCaller_Succeeds(t *testing.T) {
	builder := &fakeBuilder{tag: "img:sha"}
	rt, db := newTestRouterWithBuilder(t, builder, newFakeFetch("/tmp/checkout", nil))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/deploy-spec", bindMountDeploySpecBody))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if builder.multiCalls != 1 {
		t.Fatalf("builder.multiCalls = %d, want 1", builder.multiCalls)
	}
}
