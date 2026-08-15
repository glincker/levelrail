package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeBuilder is a hand-written fake for Builder, the same pattern
// internal/webhook's own fakeDeployer already establishes for the
// identical interface shape (this package deliberately mirrors it rather
// than importing internal/webhook's test-only type, since Go doesn't
// export test helpers across packages).
type fakeBuilder struct {
	tag string
	err error

	calls   int
	lastReq deploy.Request
}

func (f *fakeBuilder) Deploy(_ context.Context, req deploy.Request, progress func(build.ProgressEvent)) (string, error) {
	f.calls++
	f.lastReq = req
	if progress != nil {
		progress(build.ProgressEvent{Step: "fake", Completed: true})
		// A real output-carrying event too (Step-lifecycle events like
		// the one above have no Log, so internal/deploylog.Recorder never
		// persists them): TestHandleTriggerBuild_RecordsDeployAttempt_Succeeded
		// asserts this line actually lands in the persisted deploy log,
		// proving the recorder is wired through end to end.
		progress(build.ProgressEvent{Log: "fake build output", Stream: "stdout"})
	}
	if f.err != nil {
		return "", f.err
	}
	return f.tag, nil
}

// fakeFetchCall records one call's arguments, so a test can assert what a
// handler passed through without a real git clone.
type fakeFetchCall struct {
	repoURL string
	ref     string
}

// newFakeFetch returns a fetchFunc that records every call into calls and
// returns dir/cleanup/err, the same "hand-written fake, not a framework"
// pattern internal/webhook/webhook_test.go's own fetchFunc fakes use.
func newFakeFetch(calls *[]fakeFetchCall, dir string, cleanupCalled *bool, err error) fetchFunc {
	return func(_ context.Context, repoURL, ref string) (string, func(), error) {
		*calls = append(*calls, fakeFetchCall{repoURL: repoURL, ref: ref})
		if err != nil {
			return "", nil, err
		}
		return dir, func() { *cleanupCalled = true }, nil
	}
}

// newTestRouterWithBuilder builds a Router with builder wired via the
// public Option (WithBuilder, the same shape main.go itself uses) and
// fetch overridden directly on the unexported field: fetch has no public
// Option because nothing outside this package ever needs to fake git
// I/O, the same reasoning internal/webhook.Handler.fetch stays
// unexported and is only ever set through that package's own
// newTestHandler helper.
func newTestRouterWithBuilder(t *testing.T, builder Builder, fetch fetchFunc) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithBuilder(builder))
	if fetch != nil {
		rt.fetch = fetch
	}
	return rt, db
}

func TestHandleTriggerBuild_NotConfigured(t *testing.T) {
	// No WithBuilder: the route must return 501, the same "not
	// configured" shape WithSecretSetter/WithTelemetryQuerier/
	// WithAlertRules absence already share, not a 404 or a panic.
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", `{"repo_url":"https://example.com/x.git","ref":"main"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleTriggerBuild_AppNotFound(t *testing.T) {
	rt, db := newTestRouterWithBuilder(t, &fakeBuilder{tag: "levelrail/web:abc123"}, nil)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/ghost/builds", `{"repo_url":"https://example.com/x.git","ref":"main"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleTriggerBuild_MissingRepoURL(t *testing.T) {
	fb := &fakeBuilder{tag: "levelrail/web:abc123"}
	rt, db := newTestRouterWithBuilder(t, fb, nil)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", `{"ref":"main"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if fb.calls != 0 {
		t.Errorf("builder called %d times, want 0 for a rejected request", fb.calls)
	}
}

func TestHandleTriggerBuild_MissingRef(t *testing.T) {
	fb := &fakeBuilder{tag: "levelrail/web:abc123"}
	rt, db := newTestRouterWithBuilder(t, fb, nil)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", `{"repo_url":"https://example.com/x.git"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if fb.calls != 0 {
		t.Errorf("builder called %d times, want 0 for a rejected request", fb.calls)
	}
}

func TestHandleTriggerBuild_UnsupportedBuildType(t *testing.T) {
	fb := &fakeBuilder{tag: "levelrail/web:abc123"}
	var fetchCalls []fakeFetchCall
	cleanupCalled := false
	rt, db := newTestRouterWithBuilder(t, fb, newFakeFetch(&fetchCalls, t.TempDir(), &cleanupCalled, nil))
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds",
		`{"repo_url":"https://example.com/x.git","ref":"main","build":{"type":"compose"}}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
	if len(fetchCalls) != 0 {
		t.Errorf("fetch called %d times, want 0: an unsupported build type must fail before any git I/O", len(fetchCalls))
	}
	if fb.calls != 0 {
		t.Errorf("builder called %d times, want 0", fb.calls)
	}
}

func TestHandleTriggerBuild_Success(t *testing.T) {
	fb := &fakeBuilder{tag: "web-repo:main"}
	var fetchCalls []fakeFetchCall
	cleanupCalled := false
	checkoutDir := t.TempDir()
	rt, db := newTestRouterWithBuilder(t, fb, newFakeFetch(&fetchCalls, checkoutDir, &cleanupCalled, nil))
	cookie := loginTestSession(t, rt, db)

	seed := store.DesiredService{
		Name:      "web",
		Image:     "levelrail/web:old",
		Port:      3000,
		Domains:   []string{"web.example.com"},
		Env:       map[string]string{"LOG_LEVEL": "info"},
		SecretEnv: []string{"API_KEY"},
		Resources: &store.ServiceResources{MemoryBytes: 512 * 1024 * 1024, NanoCPUs: 500_000_000},
		Health: &store.ServiceHealth{
			Readiness: &store.ServiceProbe{Path: "/healthz"},
		},
	}
	if err := db.SaveDesiredService(context.Background(), seed); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds",
		`{"repo_url":"https://example.com/web.git","ref":"main","build":{"path":"./Dockerfile"}}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	if len(fetchCalls) != 1 {
		t.Fatalf("fetch called %d times, want 1", len(fetchCalls))
	}
	if fetchCalls[0].repoURL != "https://example.com/web.git" {
		t.Errorf("fetch repoURL = %q, want %q", fetchCalls[0].repoURL, "https://example.com/web.git")
	}
	if fetchCalls[0].ref != "main" {
		t.Errorf("fetch ref = %q, want %q", fetchCalls[0].ref, "main")
	}
	if !cleanupCalled {
		t.Error("fetch cleanup was not called after a successful build, checkout dir would leak")
	}

	if fb.calls != 1 {
		t.Fatalf("builder called %d times, want 1", fb.calls)
	}
	got := fb.lastReq
	if got.ServiceName != "web" {
		t.Errorf("ServiceName = %q, want %q", got.ServiceName, "web")
	}
	if got.SourceDir != checkoutDir {
		t.Errorf("SourceDir = %q, want %q", got.SourceDir, checkoutDir)
	}
	if got.CommitSHA != "main" {
		t.Errorf("CommitSHA = %q, want %q", got.CommitSHA, "main")
	}
	// No image_repo in the request body: defaults to the app's own name.
	if got.ImageRepo != "web" {
		t.Errorf("ImageRepo = %q, want %q (default to app name)", got.ImageRepo, "web")
	}
	if got.Service.Build.Type != "dockerfile" {
		t.Errorf("Service.Build.Type = %q, want %q", got.Service.Build.Type, "dockerfile")
	}
	if got.Service.Build.Path != "./Dockerfile" {
		t.Errorf("Service.Build.Path = %q, want %q", got.Service.Build.Path, "./Dockerfile")
	}
	// Everything else about the service (port, domains, env, secret env,
	// resources, health) must carry forward from the app's existing
	// desired state untouched, since the request body never repeated any
	// of it.
	if got.Service.Port != 3000 {
		t.Errorf("Service.Port = %d, want 3000", got.Service.Port)
	}
	if len(got.Service.Domains) != 1 || got.Service.Domains[0] != "web.example.com" {
		t.Errorf("Service.Domains = %v, want [web.example.com]", got.Service.Domains)
	}
	if v, ok := got.Service.Env["LOG_LEVEL"]; !ok || v.Value != "info" || v.Secret {
		t.Errorf("Service.Env[LOG_LEVEL] = %+v, want literal value %q", v, "info")
	}
	if v, ok := got.Service.Env["API_KEY"]; !ok || !v.Secret || v.Value != "" {
		t.Errorf("Service.Env[API_KEY] = %+v, want a secret marker with no value", v)
	}
	if got.Service.Resources == nil || got.Service.Resources.Memory != "512Mi" {
		t.Errorf("Service.Resources = %+v, want Memory=512Mi", got.Service.Resources)
	}
	if got.Service.Resources.CPU != 0.5 {
		t.Errorf("Service.Resources.CPU = %v, want 0.5", got.Service.Resources.CPU)
	}
	if got.Service.Health == nil || got.Service.Health.Readiness == nil || got.Service.Health.Readiness.Path != "/healthz" {
		t.Errorf("Service.Health = %+v, want Readiness.Path=/healthz", got.Service.Health)
	}

	var resp triggerBuildResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Image != "web-repo:main" {
		t.Errorf("response Image = %q, want %q", resp.Image, "web-repo:main")
	}
	if resp.App.Name != "web" {
		t.Errorf("response App.Name = %q, want %q", resp.App.Name, "web")
	}
}

// TestHandleTriggerBuild_RecordsDeployAttempt_Succeeded covers the
// second of the two build-triggering paths: unlike the plain image-tag
// path (deploys_test.go's TestHandleTriggerDeploy_RecordsDeployAttempt),
// this one has a real build step, so it should start "running" (backed
// by the recorder's live fan-out via WithDeployRecorder) and finish
// "succeeded" once fakeBuilder.Deploy returns.
func TestHandleTriggerBuild_RecordsDeployAttempt_Succeeded(t *testing.T) {
	fb := &fakeBuilder{tag: "levelrail/web:abc123"}
	var fetchCalls []fakeFetchCall
	cleanupCalled := false
	db := openTestDB(t)
	telemetryDB := newTestTelemetryDB(t)
	recorder := deploylog.NewRecorder(telemetryDB, discardLogger())
	rt := NewRouter(nil, testBrand(), db, WithBuilder(fb), WithDeployRecorder(recorder))
	rt.fetch = newFakeFetch(&fetchCalls, t.TempDir(), &cleanupCalled, nil)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds",
		`{"repo_url":"https://example.com/web.git","ref":"main"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	attempts, err := db.ListDeployAttempts(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("expected 1 deploy attempt, got %d", len(attempts))
	}
	a := attempts[0]
	if a.Status != store.DeployAttemptStatusSucceeded {
		t.Errorf("Status = %q, want %q", a.Status, store.DeployAttemptStatusSucceeded)
	}
	if a.Image != "web:main" {
		t.Errorf("Image = %q, want %q (ImageRepo:CommitSHA computed before the build, mirroring internal/deploy's own tag construction)", a.Image, "web:main")
	}
	if a.CommitSHA != "main" {
		t.Errorf("CommitSHA = %q, want %q", a.CommitSHA, "main")
	}
	if a.Source != store.DeployAttemptSourceManual {
		t.Errorf("Source = %q, want %q", a.Source, store.DeployAttemptSourceManual)
	}
	if a.FinishedAt == nil {
		t.Error("FinishedAt = nil, want set")
	}

	// fakeBuilder.Deploy invokes progress with a real output-carrying
	// event; confirm it landed in the persisted deploy log, proving the
	// recorder was actually wired through end to end, not just the
	// attempt row.
	got, err := telemetryDB.QueryDeployLog(context.Background(), a.ID)
	if err != nil {
		t.Fatalf("QueryDeployLog() error = %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one persisted log line from the fake builder's progress callback")
	}
}

func TestHandleTriggerBuild_RecordsDeployAttempt_Failed(t *testing.T) {
	fb := &fakeBuilder{err: errors.New("buildkit: boom")}
	var fetchCalls []fakeFetchCall
	cleanupCalled := false
	db := openTestDB(t)
	recorder := deploylog.NewRecorder(nil, discardLogger())
	rt := NewRouter(nil, testBrand(), db, WithBuilder(fb), WithDeployRecorder(recorder))
	rt.fetch = newFakeFetch(&fetchCalls, t.TempDir(), &cleanupCalled, nil)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds",
		`{"repo_url":"https://example.com/web.git","ref":"main"}`))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	attempts, err := db.ListDeployAttempts(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("expected 1 deploy attempt even for a failed build, got %d", len(attempts))
	}
	if attempts[0].Status != store.DeployAttemptStatusFailed {
		t.Errorf("Status = %q, want %q", attempts[0].Status, store.DeployAttemptStatusFailed)
	}
	if attempts[0].Error != "buildkit: boom" {
		t.Errorf("Error = %q, want %q", attempts[0].Error, "buildkit: boom")
	}
}

func TestHandleTriggerBuild_NoRecorderConfigured_StillRecordsAttemptWithoutLog(t *testing.T) {
	// No WithDeployRecorder: handleTriggerBuild must still degrade
	// gracefully (build.SlogProgress fallback, matching this package's
	// pre-existing behavior) rather than fail the whole request, and
	// still records the attempt row itself since rt.deployAttempts is
	// always available (it's part of the core Store interface, not an
	// optional plug-in).
	fb := &fakeBuilder{tag: "levelrail/web:abc123"}
	var fetchCalls []fakeFetchCall
	cleanupCalled := false
	rt, db := newTestRouterWithBuilder(t, fb, newFakeFetch(&fetchCalls, t.TempDir(), &cleanupCalled, nil))
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds",
		`{"repo_url":"https://example.com/web.git","ref":"main"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	attempts, err := db.ListDeployAttempts(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("expected 1 deploy attempt, got %d", len(attempts))
	}
	if attempts[0].Status != store.DeployAttemptStatusSucceeded {
		t.Errorf("Status = %q, want %q", attempts[0].Status, store.DeployAttemptStatusSucceeded)
	}
}

func TestHandleTriggerBuild_DefaultBuildTypeAndImageRepoOverride(t *testing.T) {
	fb := &fakeBuilder{tag: "custom-repo:main"}
	var fetchCalls []fakeFetchCall
	cleanupCalled := false
	rt, db := newTestRouterWithBuilder(t, fb, newFakeFetch(&fetchCalls, t.TempDir(), &cleanupCalled, nil))
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds",
		`{"repo_url":"https://example.com/web.git","ref":"main","image_repo":"custom-repo"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if fb.lastReq.ImageRepo != "custom-repo" {
		t.Errorf("ImageRepo = %q, want %q", fb.lastReq.ImageRepo, "custom-repo")
	}
	if fb.lastReq.Service.Build.Type != "dockerfile" {
		t.Errorf("Service.Build.Type = %q, want default %q", fb.lastReq.Service.Build.Type, "dockerfile")
	}
}

func TestHandleTriggerBuild_FetchFailure(t *testing.T) {
	fb := &fakeBuilder{tag: "levelrail/web:abc123"}
	var fetchCalls []fakeFetchCall
	cleanupCalled := false
	rt, db := newTestRouterWithBuilder(t, fb, newFakeFetch(&fetchCalls, "", &cleanupCalled, errors.New("clone failed: repository not found")))
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds",
		`{"repo_url":"https://example.com/ghost.git","ref":"main"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if fb.calls != 0 {
		t.Errorf("builder called %d times, want 0 when fetch fails", fb.calls)
	}
}

func TestHandleTriggerBuild_BuildFailure_ReturnsServerErrorNotLeaky(t *testing.T) {
	fb := &fakeBuilder{err: errors.New("buildkit: internal socket path /var/run/secret-thing failed")}
	var fetchCalls []fakeFetchCall
	cleanupCalled := false
	rt, db := newTestRouterWithBuilder(t, fb, newFakeFetch(&fetchCalls, t.TempDir(), &cleanupCalled, nil))
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds",
		`{"repo_url":"https://example.com/web.git","ref":"main"}`))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-thing") {
		t.Errorf("response body leaked internal error detail: %q", rec.Body.String())
	}
	if !cleanupCalled {
		t.Error("cleanup was not called after a failed build, checkout dir would leak")
	}
}

func TestHandleTriggerBuild_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouterWithBuilder(t, &fakeBuilder{}, nil)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/builds", strings.NewReader(`{"repo_url":"x","ref":"main"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleTriggerBuild_InvalidBody(t *testing.T) {
	rt, db := newTestRouterWithBuilder(t, &fakeBuilder{}, nil)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", `not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
