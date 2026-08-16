package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeBuilder is a hand-written fake for Builder. calls/lastReq stay
// plain, mutex-guarded fields for git_webhook_test.go's synchronous
// &fakeBuilder{} literals. notify (set only via newFakeBuilder) is for
// this file's own async callers: handleTriggerBuild now calls Deploy
// from a background goroutine, so a test has to synchronize on
// something real rather than reading calls/lastReq directly.
type fakeBuilder struct {
	tag string
	err error

	mu      sync.Mutex
	calls   int
	lastReq deploy.Request

	notify chan deploy.Request

	// multiCalls, lastMultiReq, multiOutcomes, and multiErr back
	// DeploySpec, apps_multi_test.go's own fake surface: multiOutcomes
	// (set explicitly) takes priority when non-nil, otherwise DeploySpec
	// synthesizes one successful ServiceOutcome per service key from tag,
	// the same default-success shape Deploy above already has via tag/err.
	// DeploySpec is called synchronously from handleDeploySpec (unlike
	// Deploy above), so these need no mutex.
	multiCalls    int
	lastMultiReq  deploy.MultiRequest
	multiOutcomes []deploy.ServiceOutcome
	multiErr      error
}

func (f *fakeBuilder) DeploySpec(_ context.Context, req deploy.MultiRequest, progress func(string, build.ProgressEvent)) ([]deploy.ServiceOutcome, error) {
	f.multiCalls++
	f.lastMultiReq = req
	if f.multiErr != nil {
		return nil, f.multiErr
	}
	if f.multiOutcomes != nil {
		return f.multiOutcomes, nil
	}

	keys := make([]string, 0, len(req.Services))
	for k := range req.Services {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]deploy.ServiceOutcome, 0, len(keys))
	for _, k := range keys {
		if progress != nil {
			progress(k, build.ProgressEvent{Step: "fake", Completed: true})
		}
		out = append(out, deploy.ServiceOutcome{ServiceKey: k, ServiceName: req.AppName + "-" + k, Image: f.tag})
	}
	return out, nil
}

func newFakeBuilder(tag string, err error) *fakeBuilder {
	return &fakeBuilder{tag: tag, err: err, notify: make(chan deploy.Request, 4)}
}

func (f *fakeBuilder) Deploy(_ context.Context, req deploy.Request, progress func(build.ProgressEvent)) (string, error) {
	if progress != nil {
		progress(build.ProgressEvent{Step: "fake", Completed: true})
		// A real output-carrying event too (Step-lifecycle events like
		// the one above have no Log, so internal/deploylog.Recorder never
		// persists them): TestHandleTriggerBuild_RecordsDeployAttempt_Succeeded
		// asserts this line actually lands in the persisted deploy log,
		// proving the recorder is wired through end to end.
		progress(build.ProgressEvent{Log: "fake build output", Stream: "stdout"})
	}
	f.mu.Lock()
	f.calls++
	f.lastReq = req
	f.mu.Unlock()
	if f.notify != nil {
		f.notify <- req
	}
	if f.err != nil {
		return "", f.err
	}
	return f.tag, nil
}

// awaitCall waits up to a short deadline for Deploy to have been called,
// rather than a fixed sleep: handleTriggerBuild starts it in a goroutine
// specifically so the HTTP response doesn't wait for it, so a test
// asserting it happened has to synchronize on something real.
func (f *fakeBuilder) awaitCall(t *testing.T) deploy.Request {
	t.Helper()
	select {
	case req := <-f.notify:
		return req
	case <-time.After(2 * time.Second):
		t.Fatal("Deploy was not called within the deadline")
		return deploy.Request{}
	}
}

// assertNotCalled checks, without blocking, that Deploy has not been
// called: safe to use right after ServeHTTP returns only when the
// handler is known to have rejected the request before ever starting the
// build goroutine (no goroutine means nothing can race with this read).
func (f *fakeBuilder) assertNotCalled(t *testing.T) {
	t.Helper()
	select {
	case req := <-f.notify:
		t.Errorf("Deploy called unexpectedly with %+v, want not called", req)
	default:
	}
}

// fakeFetchCall records one call's arguments, so a test can assert what
// the build goroutine passed through without a real git clone.
type fakeFetchCall struct {
	repoURL, ref, token string
}

// fakeFetch is a hand-written fetchFunc fake: fetch now runs inside
// handleTriggerBuild's background goroutine, so both the call record and
// the cleanup signal go over buffered channels rather than plain fields,
// the same reasoning fakeBuilder's own doc comment gives.
type fakeFetch struct {
	dir string
	err error

	calls    chan fakeFetchCall
	cleanups chan struct{}
}

func newFakeFetch(dir string, err error) *fakeFetch {
	return &fakeFetch{dir: dir, err: err, calls: make(chan fakeFetchCall, 4), cleanups: make(chan struct{}, 4)}
}

func (f *fakeFetch) fetch(_ context.Context, repoURL, ref, token string) (string, func(), error) {
	f.calls <- fakeFetchCall{repoURL: repoURL, ref: ref, token: token}
	if f.err != nil {
		return "", nil, f.err
	}
	return f.dir, func() { f.cleanups <- struct{}{} }, nil
}

func (f *fakeFetch) awaitCall(t *testing.T) fakeFetchCall {
	t.Helper()
	select {
	case c := <-f.calls:
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("fetch was not called within the deadline")
		return fakeFetchCall{}
	}
}

func (f *fakeFetch) assertNotCalled(t *testing.T) {
	t.Helper()
	select {
	case c := <-f.calls:
		t.Errorf("fetch called unexpectedly with %+v, want not called", c)
	default:
	}
}

func (f *fakeFetch) awaitCleanup(t *testing.T) {
	t.Helper()
	select {
	case <-f.cleanups:
	case <-time.After(2 * time.Second):
		t.Fatal("fetch cleanup was not called within the deadline")
	}
}

// awaitDeployAttemptFinished polls db for id's attempt until FinishedAt is
// set, since handleTriggerBuild's build now runs in a background
// goroutine and the test otherwise has no way to know when finishAttempt
// (deploy_attempts.go) has actually persisted the terminal status.
func awaitDeployAttemptFinished(t *testing.T, db *store.DB, id string) store.DeployAttempt {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		a, err := db.GetDeployAttempt(context.Background(), id)
		if err != nil {
			t.Fatalf("GetDeployAttempt(%q) error = %v", id, err)
		}
		if a.FinishedAt != nil {
			return *a
		}
		if time.Now().After(deadline) {
			t.Fatalf("deploy attempt %q did not finish within the deadline", id)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// newTestRouterWithBuilder builds a Router with builder wired via the
// public Option (WithBuilder, the same shape main.go itself uses) and
// fetch overridden directly on the unexported field: fetch has no public
// Option because nothing outside this package ever needs to fake git
// I/O, the same reasoning internal/webhook.Handler.fetch stays
// unexported and is only ever set through that package's own
// newTestHandler helper.
func newTestRouterWithBuilder(t *testing.T, builder Builder, fetch *fakeFetch) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithBuilder(builder))
	if fetch != nil {
		rt.fetch = fetch.fetch
	}
	return rt, db
}

// seedWebApp saves the standard "web" desired service fixture nearly
// every build-trigger test needs before POSTing.
func seedWebApp(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
}

// postTriggerBuildAccepted POSTs body to the build-trigger route, fails
// the test unless the response is 202, and decodes the deploy attempt id.
// Returns the raw recorder too, for callers that also need to inspect the
// response body (e.g. asserting a secret was never echoed back).
func postTriggerBuildAccepted(t *testing.T, rt *Router, cookie *http.Cookie, body string) (*httptest.ResponseRecorder, triggerBuildResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", body))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var resp triggerBuildResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return rec, resp
}

// postTriggerBuildBearerAccepted is postTriggerBuildAccepted's bearer-token
// counterpart, for tests exercising a scoped API token instead of a
// session cookie.
func postTriggerBuildBearerAccepted(t *testing.T, rt *Router, bearerToken, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/builds", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
}

// newPrivateRepoAuthRouter builds a Router wired with a connected,
// installed GitHub App (seedInstalledGitHubApp) and a fake installation
// token client, the shared setup every PrivateRepoAuth test in this file
// needs before exercising tokenForRepo's own gating logic.
func newPrivateRepoAuthRouter(t *testing.T, fb *fakeBuilder, fetch *fakeFetch, fakeClient *fakeGitHubAppClient) (*Router, *store.DB) {
	t.Helper()
	fakeSecrets := newFakeGitHubAppSecrets()
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithBuilder(fb), WithGitHubAppSecrets(fakeSecrets))
	rt.githubAppClient = fakeClient
	rt.fetch = fetch.fetch
	seedInstalledGitHubApp(t, db, fakeSecrets)
	return rt, db
}

func TestHandleTriggerBuild_NotConfigured(t *testing.T) {
	// No WithBuilder: the route must return 501, the same "not
	// configured" shape WithSecretSetter/WithTelemetryQuerier/
	// WithAlertRules absence already share, not a 404 or a panic.
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	seedWebApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", `{"repo_url":"https://example.com/x.git","ref":"main"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleTriggerBuild_AppNotFound(t *testing.T) {
	rt, db := newTestRouterWithBuilder(t, newFakeBuilder("levelrail/web:abc123", nil), nil)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/ghost/builds", `{"repo_url":"https://example.com/x.git","ref":"main"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleTriggerBuild_MissingRepoURL(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	rt, db := newTestRouterWithBuilder(t, fb, nil)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", `{"ref":"main"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	fb.assertNotCalled(t)
}

func TestHandleTriggerBuild_MissingRef(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	rt, db := newTestRouterWithBuilder(t, fb, nil)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", `{"repo_url":"https://example.com/x.git"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	fb.assertNotCalled(t)
}

func TestHandleTriggerBuild_UnsupportedBuildType(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds",
		`{"repo_url":"https://example.com/x.git","ref":"main","build":{"type":"compose"}}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
	fetch.assertNotCalled(t)
	fb.assertNotCalled(t)
}

func TestHandleTriggerBuild_Success(t *testing.T) {
	fb := newFakeBuilder("web-repo:main", nil)
	checkoutDir := t.TempDir()
	fetch := newFakeFetch(checkoutDir, nil)
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
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

	_, resp := postTriggerBuildAccepted(t, rt, cookie, `{"repo_url":"https://example.com/web.git","ref":"main","build":{"path":"./Dockerfile"}}`)
	if resp.ID == "" {
		t.Fatal("response ID is empty, want a deploy attempt id returned immediately")
	}

	fc := fetch.awaitCall(t)
	if fc.repoURL != "https://example.com/web.git" {
		t.Errorf("fetch repoURL = %q, want %q", fc.repoURL, "https://example.com/web.git")
	}
	if fc.ref != "main" {
		t.Errorf("fetch ref = %q, want %q", fc.ref, "main")
	}
	if fc.token != "" {
		t.Errorf("fetch token = %q, want empty (no github app configured)", fc.token)
	}

	got := fb.awaitCall(t)
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

	attempt := awaitDeployAttemptFinished(t, db, resp.ID)
	if attempt.Status != store.DeployAttemptStatusSucceeded {
		t.Errorf("attempt Status = %q, want %q", attempt.Status, store.DeployAttemptStatusSucceeded)
	}
	fetch.awaitCleanup(t)
}

// TestHandleTriggerBuild_PreservesCustomLabels is a regression test:
// specServiceFromDesired originally reconstructed every field a manual
// rebuild's request body doesn't repeat (env, secret env, resources,
// health) except Labels, so a service with custom labels set through
// the create/update API or LabelsEditor.tsx would have them silently
// dropped the next time someone triggered POST .../builds, since that
// path does a full-replace SaveDesiredService like every other write.
func TestHandleTriggerBuild_PreservesCustomLabels(t *testing.T) {
	fb := newFakeBuilder("web-repo:main", nil)
	checkoutDir := t.TempDir()
	fetch := newFakeFetch(checkoutDir, nil)
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
	cookie := loginTestSession(t, rt, db)

	seed := store.DesiredService{
		Name:   "web",
		Image:  "levelrail/web:old",
		Port:   3000,
		Labels: map[string]string{"team": "infra", "tier": "backend"},
	}
	if err := db.SaveDesiredService(context.Background(), seed); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	postTriggerBuildAccepted(t, rt, cookie, `{"repo_url":"https://example.com/web.git","ref":"main","build":{"path":"./Dockerfile"}}`)

	got := fb.awaitCall(t).Service.Labels
	want := map[string]string{"team": "infra", "tier": "backend"}
	if len(got) != len(want) {
		t.Fatalf("Service.Labels = %+v, want %+v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("Service.Labels[%q] = %q, want %q", k, got[k], v)
		}
	}

	// The stored desired state itself must also still carry the labels
	// after the rebuild's own SaveDesiredService write, not just the
	// build request in flight. Deploy runs asynchronously, so wait for
	// the store write internal/deploy.Pipeline.Deploy performs (here,
	// fakeBuilder never touches the store, so this only proves the seed
	// value survived, but a real builder would have overwritten it by
	// the time the deploy attempt finishes; fake builds never persist,
	// so this asserts the seed itself was never clobbered by this
	// request's own SaveDesiredService, which it never calls).
	stored, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService: %v", err)
	}
	if len(stored.Labels) != len(want) {
		t.Fatalf("stored Labels = %+v, want %+v", stored.Labels, want)
	}
}

// TestHandleTriggerBuild_RecordsDeployAttempt_Succeeded covers the
// second of the two build-triggering paths: unlike the plain image-tag
// path (deploys_test.go's TestHandleTriggerDeploy_RecordsDeployAttempt),
// this one has a real build step, so it should start "running" (visible
// immediately in the response and in GET .../deploy-attempts, backed by
// the recorder's live fan-out via WithDeployRecorder) and finish
// "succeeded" once fakeBuilder.Deploy returns.
func TestHandleTriggerBuild_RecordsDeployAttempt_Succeeded(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	db := openTestDB(t)
	telemetryDB := newTestTelemetryDB(t)
	recorder := deploylog.NewRecorder(telemetryDB, discardLogger())
	rt := NewRouter(nil, testBrand(), db, WithBuilder(fb), WithDeployRecorder(recorder))
	rt.fetch = fetch.fetch
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	_, resp := postTriggerBuildAccepted(t, rt, cookie, `{"repo_url":"https://example.com/web.git","ref":"main"}`)

	// Immediately after the handler returns, before the build goroutine
	// has necessarily run at all, the attempt row must already exist and
	// be "running": that is the entire point of returning early.
	running, err := db.GetDeployAttempt(context.Background(), resp.ID)
	if err != nil {
		t.Fatalf("GetDeployAttempt() error = %v", err)
	}
	if running.Status != store.DeployAttemptStatusRunning {
		t.Errorf("Status immediately after response = %q, want %q", running.Status, store.DeployAttemptStatusRunning)
	}

	a := awaitDeployAttemptFinished(t, db, resp.ID)
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
	fb := newFakeBuilder("", errors.New("buildkit: boom"))
	fetch := newFakeFetch(t.TempDir(), nil)
	db := openTestDB(t)
	recorder := deploylog.NewRecorder(nil, discardLogger())
	rt := NewRouter(nil, testBrand(), db, WithBuilder(fb), WithDeployRecorder(recorder))
	rt.fetch = fetch.fetch
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	_, resp := postTriggerBuildAccepted(t, rt, cookie, `{"repo_url":"https://example.com/web.git","ref":"main"}`)

	attempt := awaitDeployAttemptFinished(t, db, resp.ID)
	if attempt.Status != store.DeployAttemptStatusFailed {
		t.Errorf("Status = %q, want %q", attempt.Status, store.DeployAttemptStatusFailed)
	}
	if attempt.Error != "buildkit: boom" {
		t.Errorf("Error = %q, want %q", attempt.Error, "buildkit: boom")
	}
	fetch.awaitCleanup(t)
}

func TestHandleTriggerBuild_NoRecorderConfigured_StillRecordsAttemptWithoutLog(t *testing.T) {
	// No WithDeployRecorder: handleTriggerBuild must still degrade
	// gracefully (build.SlogProgress fallback, matching this package's
	// pre-existing behavior) rather than fail the whole request, and
	// still records the attempt row itself since rt.deployAttempts is
	// always available (it's part of the core Store interface, not an
	// optional plug-in).
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	_, resp := postTriggerBuildAccepted(t, rt, cookie, `{"repo_url":"https://example.com/web.git","ref":"main"}`)

	attempt := awaitDeployAttemptFinished(t, db, resp.ID)
	if attempt.Status != store.DeployAttemptStatusSucceeded {
		t.Errorf("Status = %q, want %q", attempt.Status, store.DeployAttemptStatusSucceeded)
	}
}

func TestHandleTriggerBuild_DefaultBuildTypeAndImageRepoOverride(t *testing.T) {
	fb := newFakeBuilder("custom-repo:main", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	postTriggerBuildAccepted(t, rt, cookie, `{"repo_url":"https://example.com/web.git","ref":"main","image_repo":"custom-repo"}`)
	got := fb.awaitCall(t)
	if got.ImageRepo != "custom-repo" {
		t.Errorf("ImageRepo = %q, want %q", got.ImageRepo, "custom-repo")
	}
	if got.Service.Build.Type != "dockerfile" {
		t.Errorf("Service.Build.Type = %q, want default %q", got.Service.Build.Type, "dockerfile")
	}
}

func TestHandleTriggerBuild_FetchFailure(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fetch := newFakeFetch("", errors.New("clone failed: repository not found"))
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	// A fetch failure is only discoverable once the background goroutine
	// runs it, after the response has already gone out as 202: unlike
	// the old synchronous handler, this is no longer a 400 the caller
	// sees directly.
	_, resp := postTriggerBuildAccepted(t, rt, cookie, `{"repo_url":"https://example.com/ghost.git","ref":"main"}`)

	attempt := awaitDeployAttemptFinished(t, db, resp.ID)
	if attempt.Status != store.DeployAttemptStatusFailed {
		t.Errorf("Status = %q, want %q", attempt.Status, store.DeployAttemptStatusFailed)
	}
	fb.assertNotCalled(t)
}

func TestHandleTriggerBuild_BuildFailure_ReturnsAcceptedNotLeaky(t *testing.T) {
	fb := newFakeBuilder("", errors.New("buildkit: internal socket path /var/run/secret-thing failed"))
	fetch := newFakeFetch(t.TempDir(), nil)
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	rec, resp := postTriggerBuildAccepted(t, rt, cookie, `{"repo_url":"https://example.com/web.git","ref":"main"}`)
	if strings.Contains(rec.Body.String(), "secret-thing") {
		t.Errorf("response body leaked internal error detail: %q", rec.Body.String())
	}

	// The build failure detail is only ever visible in the deploy
	// attempt row (an operator-facing surface, not a raw API response
	// body), matching internal/webhook.Handler.ServeHTTP's own "500 with
	// a non-leaky message" reasoning for an identical failure.
	attempt := awaitDeployAttemptFinished(t, db, resp.ID)
	if attempt.Status != store.DeployAttemptStatusFailed {
		t.Errorf("Status = %q, want %q", attempt.Status, store.DeployAttemptStatusFailed)
	}
	if attempt.Error != "buildkit: internal socket path /var/run/secret-thing failed" {
		t.Errorf("Error = %q, want the full error persisted in the attempt row", attempt.Error)
	}
	fetch.awaitCleanup(t)
}

func TestHandleTriggerBuild_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouterWithBuilder(t, newFakeBuilder("", nil), nil)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/builds", strings.NewReader(`{"repo_url":"x","ref":"main"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestHandleTriggerBuild_RailpackAccepted proves the manual build trigger
// no longer rejects build.type: railpack: internal/deploy.Pipeline.Deploy
// has a real deployRailpack case, only handleTriggerBuild's own
// pre-fetch rejection was blocking it.
func TestHandleTriggerBuild_RailpackAccepted(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:railpack1", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	postTriggerBuildAccepted(t, rt, cookie, `{"repo_url":"https://example.com/web.git","ref":"main","build":{"type":"railpack"}}`)
	got := fb.awaitCall(t)
	if got.Service.Build.Type != "railpack" {
		t.Errorf("Service.Build.Type = %q, want %q", got.Service.Build.Type, "railpack")
	}
}

// TestHandleTriggerBuild_RailpackWithPathRejected proves a caller-supplied
// build.path for build.type: railpack fails loudly rather than being
// silently discarded: build.RailpackRequest (internal/deploy's own
// deployRailpack) has no path field at all, so a value here would
// otherwise vanish with no signal to the caller that it did nothing.
func TestHandleTriggerBuild_RailpackWithPathRejected(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:railpack1", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds",
		`{"repo_url":"https://example.com/web.git","ref":"main","build":{"type":"railpack","path":"./Dockerfile"}}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	fetch.assertNotCalled(t)
	fb.assertNotCalled(t)
}

// TestHandleTriggerBuild_StaticAccepted proves the manual build trigger
// no longer rejects build.type: static.
func TestHandleTriggerBuild_StaticAccepted(t *testing.T) {
	// fakeBuilder.Deploy returns an absolute-looking path, exactly the
	// shape internal/deploy.Pipeline.deployStatic really returns (the
	// served-from directory, not an image tag).
	fb := newFakeBuilder("/var/lib/levelrail-data/static-sites/web/main", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Domains: []string{"web.example.com"}}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec, _ := postTriggerBuildAccepted(t, rt, cookie, `{"repo_url":"https://example.com/web.git","ref":"main","build":{"type":"static"}}`)
	got := fb.awaitCall(t)
	if got.Service.Build.Type != "static" {
		t.Errorf("Service.Build.Type = %q, want %q", got.Service.Build.Type, "static")
	}

	body := rec.Body.String()
	if strings.Contains(body, "var/lib/levelrail-data") {
		t.Errorf("response body leaked the static site's local filesystem path: %s", body)
	}
}

// TestHandleTriggerBuild_ComposeStillRejected proves build.type: compose
// specifically (not every non-dockerfile type, now that railpack and
// static are accepted above) still gets the original 501, matching
// internal/deploy.Pipeline.Deploy's own compose case, which still
// returns "not yet supported".
func TestHandleTriggerBuild_ComposeStillRejected(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds",
		`{"repo_url":"https://example.com/web.git","ref":"main","build":{"type":"compose"}}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
	fb.assertNotCalled(t)
}

// TestHandleTriggerBuild_UnrecognizedBuildTypeRejected proves a
// build.type this control plane has never heard of fails as a plain 400
// (a caller mistake), distinct from compose's 501 (a real, named
// capability gap).
func TestHandleTriggerBuild_UnrecognizedBuildTypeRejected(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds",
		`{"repo_url":"https://example.com/web.git","ref":"main","build":{"type":"nixpacks"}}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	fb.assertNotCalled(t)
}

func TestHandleTriggerBuild_InvalidBody(t *testing.T) {
	rt, db := newTestRouterWithBuilder(t, newFakeBuilder("", nil), nil)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", `not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestIsGitHubHTTPSRepoURL covers the host/scheme matching tokenForRepo
// relies on to decide whether minting a GitHub App installation token is
// even worth attempting.
func TestIsGitHubHTTPSRepoURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://github.com/acme/widgets.git", true},
		{"https://github.com/acme/widgets", true},
		{"http://github.com/acme/widgets.git", false},
		{"https://gitlab.com/acme/widgets.git", false},
		{"git@github.com:acme/widgets.git", false},
		{"not a url", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isGitHubHTTPSRepoURL(c.url); got != c.want {
			t.Errorf("isGitHubHTTPSRepoURL(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

// TestHandleTriggerBuild_PrivateRepoAuth_MintsAndUsesToken proves the
// real security-relevant fix this task adds: a github.com repo, with a
// connected and installed GitHub App, gets its clone authenticated with
// a freshly minted installation token, exactly the credential
// GitHubAppRepoPicker.tsx's own picker implies is already wired in.
func TestHandleTriggerBuild_PrivateRepoAuth_MintsAndUsesToken(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	fakeClient := &fakeGitHubAppClient{mintToken: githubapp.InstallationToken{Token: "ghs_installtoken"}}
	rt, db := newPrivateRepoAuthRouter(t, fb, fetch, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	rec, resp := postTriggerBuildAccepted(t, rt, cookie, `{"repo_url":"https://github.com/acme/widgets.git","ref":"main"}`)

	fc := fetch.awaitCall(t)
	if fc.token != "ghs_installtoken" {
		t.Errorf("fetch token = %q, want the minted installation token %q", fc.token, "ghs_installtoken")
	}
	if fc.repoURL != "https://github.com/acme/widgets.git" {
		t.Errorf("fetch repoURL = %q, want %q", fc.repoURL, "https://github.com/acme/widgets.git")
	}
	if fakeClient.gotInstallID != 7 {
		t.Errorf("MintInstallationToken called with installationID = %d, want 7", fakeClient.gotInstallID)
	}
	if strings.Contains(rec.Body.String(), "ghs_installtoken") {
		t.Error("response body leaked the installation token, must never be echoed back")
	}
	awaitDeployAttemptFinished(t, db, resp.ID)
}

// TestHandleTriggerBuild_PrivateRepoAuth_DeployOnlyTokenNeverMintsToken
// proves AbilityDeploy alone (this route's own gate) is not enough to
// authorize minting a live GitHub App installation token: repoURL is
// fully caller-controlled and need not have anything to do with the app
// being built, so an unscoped mint here would let a CI token meant only
// to trigger builds read any private repo the org's installation can
// reach. The build itself still succeeds (AbilityDeploy is enough to
// trigger a build), just against an unauthenticated clone.
func TestHandleTriggerBuild_PrivateRepoAuth_DeployOnlyTokenNeverMintsToken(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	fakeClient := &fakeGitHubAppClient{mintToken: githubapp.InstallationToken{Token: "ghs_installtoken"}}
	rt, db := newPrivateRepoAuthRouter(t, fb, fetch, fakeClient)
	seedWebApp(t, db)
	const plaintext = "deploy-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_deploy", Name: "deployer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityDeploy}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	postTriggerBuildBearerAccepted(t, rt, plaintext, `{"repo_url":"https://github.com/acme/widgets.git","ref":"main"}`)

	fc := fetch.awaitCall(t)
	if fc.token != "" {
		t.Errorf("fetch token = %q, want empty: a plain deploy token must not mint or use a live installation token", fc.token)
	}
	if fakeClient.gotInstallID != 0 {
		t.Errorf("MintInstallationToken called with installationID = %d, want 0 (never called)", fakeClient.gotInstallID)
	}
}

// TestHandleTriggerBuild_PrivateRepoAuth_ReadSensitiveTokenMintsToken
// proves the fix isn't overly restrictive: a bearer token explicitly
// scoped with AbilityReadSensitive (not just a session) still gets a
// minted installation token, the same tier handleListGitHubAppRepos
// itself requires for the equivalent disclosure.
func TestHandleTriggerBuild_PrivateRepoAuth_ReadSensitiveTokenMintsToken(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	fakeClient := &fakeGitHubAppClient{mintToken: githubapp.InstallationToken{Token: "ghs_installtoken"}}
	rt, db := newPrivateRepoAuthRouter(t, fb, fetch, fakeClient)
	seedWebApp(t, db)
	const plaintext = "read-sensitive-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_rs", Name: "ci", TokenHash: hashToken(plaintext), Abilities: []string{AbilityDeploy, AbilityReadSensitive}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	postTriggerBuildBearerAccepted(t, rt, plaintext, `{"repo_url":"https://github.com/acme/widgets.git","ref":"main"}`)

	fc := fetch.awaitCall(t)
	if fc.token != "ghs_installtoken" {
		t.Errorf("fetch token = %q, want the minted installation token: a token explicitly scoped with AbilityReadSensitive should still get one", fc.token)
	}
}

// TestHandleTriggerBuild_PrivateRepoAuth_NotConnectedFallsBackUnauthenticated
// proves a github.com repo URL with no GitHub App connected at all still
// clones (unauthenticated, the pre-existing behavior for a public repo)
// rather than failing the build outright: tokenForRepo's own doc comment
// is explicit that a manual build trigger's fetch must never hard-fail
// only because GitHub App auth isn't configured.
func TestHandleTriggerBuild_PrivateRepoAuth_NotConnectedFallsBackUnauthenticated(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	postTriggerBuildAccepted(t, rt, cookie, `{"repo_url":"https://github.com/acme/widgets.git","ref":"main"}`)

	fc := fetch.awaitCall(t)
	if fc.token != "" {
		t.Errorf("fetch token = %q, want empty (no github app connection exists at all)", fc.token)
	}
}

// TestHandleTriggerBuild_PrivateRepoAuth_NonGitHubHostNeverMintsToken
// proves a non-github.com repo URL never even attempts to mint a token,
// even with a fully connected and installed GitHub App: a token minted
// for GitHub can never authenticate a clone against a different host.
func TestHandleTriggerBuild_PrivateRepoAuth_NonGitHubHostNeverMintsToken(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	fakeClient := &fakeGitHubAppClient{mintToken: githubapp.InstallationToken{Token: "ghs_installtoken"}}
	rt, db := newPrivateRepoAuthRouter(t, fb, fetch, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	postTriggerBuildAccepted(t, rt, cookie, `{"repo_url":"https://gitlab.com/acme/widgets.git","ref":"main"}`)

	fc := fetch.awaitCall(t)
	if fc.token != "" {
		t.Errorf("fetch token = %q, want empty for a non-github.com host", fc.token)
	}
	if fakeClient.gotInstallID != 0 {
		t.Errorf("MintInstallationToken was called (installationID = %d), want never called for a non-github.com host", fakeClient.gotInstallID)
	}
}

// TestHandleTriggerBuild_PrivateRepoAuth_MintErrorFallsBackUnauthenticated
// proves a real minting error (private key resolve failure, upstream
// GitHub error, anything other than "not connected"/"not installed")
// still degrades to an unauthenticated clone rather than failing the
// whole build: tokenForRepo's own doc comment is explicit that the
// public-repo path must keep working regardless.
func TestHandleTriggerBuild_PrivateRepoAuth_MintErrorFallsBackUnauthenticated(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	fakeClient := &fakeGitHubAppClient{mintErr: errFakeSecretNotSet}
	rt, db := newPrivateRepoAuthRouter(t, fb, fetch, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	postTriggerBuildAccepted(t, rt, cookie, `{"repo_url":"https://github.com/acme/widgets.git","ref":"main"}`)

	fc := fetch.awaitCall(t)
	if fc.token != "" {
		t.Errorf("fetch token = %q, want empty after a minting error", fc.token)
	}
	fb.awaitCall(t)
}
