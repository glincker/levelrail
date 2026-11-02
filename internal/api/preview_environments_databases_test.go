package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakePreviewDBRuntime is a hand-written fake docker.Runtime, the same
// convention fakeExecAppRuntime (exec_test.go) and
// internal/reconcile/database's own fakeRuntime already establish for
// this interface: every method is a compile-satisfying stub except the
// three teardownOneEphemeralDatabase actually calls (InspectByName,
// Stop, Remove), which are stateful and configurable per test.
type fakePreviewDBRuntime struct {
	containers map[string]*docker.ContainerState
	stopErr    error
	removeErr  error
}

func newFakePreviewDBRuntime() *fakePreviewDBRuntime {
	return &fakePreviewDBRuntime{containers: map[string]*docker.ContainerState{}}
}

func (f *fakePreviewDBRuntime) seed(name string, running bool) {
	f.containers[name] = &docker.ContainerState{ID: "c-" + name, Name: name, Running: running}
}

func (f *fakePreviewDBRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	cs, ok := f.containers[name]
	if !ok {
		return nil, nil
	}
	cp := *cs
	return &cp, nil
}

func (f *fakePreviewDBRuntime) Stop(_ context.Context, id string, _ time.Duration) error {
	if f.stopErr != nil {
		return f.stopErr
	}
	for _, cs := range f.containers {
		if cs.ID == id {
			cs.Running = false
		}
	}
	return nil
}

func (f *fakePreviewDBRuntime) Remove(_ context.Context, id string, _ bool) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	for name, cs := range f.containers {
		if cs.ID == id {
			delete(f.containers, name)
		}
	}
	return nil
}

func (f *fakePreviewDBRuntime) Create(context.Context, docker.ContainerSpec) (string, error) {
	return "", nil
}
func (f *fakePreviewDBRuntime) Start(context.Context, string) error { return nil }
func (f *fakePreviewDBRuntime) Events(context.Context) (<-chan docker.Event, <-chan error) {
	return nil, nil
}
func (f *fakePreviewDBRuntime) ListImages(context.Context, string) ([]docker.ImageInfo, error) {
	return nil, nil
}
func (f *fakePreviewDBRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return nil, nil
}
func (f *fakePreviewDBRuntime) UpdateResources(context.Context, string, docker.Resources) error {
	return nil
}
func (f *fakePreviewDBRuntime) EnsureVolume(context.Context, string) error { return nil }
func (f *fakePreviewDBRuntime) EnsureNetwork(context.Context, string) (string, error) {
	return "", nil
}
func (f *fakePreviewDBRuntime) RemoveNetwork(context.Context, string) error          { return nil }
func (f *fakePreviewDBRuntime) NetworkConnect(context.Context, string, string) error { return nil }
func (f *fakePreviewDBRuntime) NetworkDisconnect(context.Context, string, string, bool) error {
	return nil
}
func (f *fakePreviewDBRuntime) ListNetworksByPrefix(context.Context, string) ([]docker.NetworkInfo, error) {
	return nil, nil
}
func (f *fakePreviewDBRuntime) Exec(context.Context, string, []string) (io.ReadCloser, error) {
	return nil, nil
}
func (f *fakePreviewDBRuntime) ExecWithInput(context.Context, string, []string, io.Reader) (io.ReadCloser, error) {
	return nil, nil
}

// setUpPreviewAppWithDatabase is setUpPreviewApp plus a single
// ephemeralInPreviews database ("main", postgres) declared on the
// connected git source, wired to a fakePreviewDBRuntime so teardown
// tests can assert real container removal without a live Docker daemon.
func setUpPreviewAppWithDatabase(t *testing.T) (rt *Router, db *store.DB, secret string, runtime *fakePreviewDBRuntime) {
	t.Helper()
	rt, db, secret, _ = setUpPreviewApp(t)

	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source",
		`{"repo_url":"https://github.com/org/web.git","branch":"main","databases":{"main":{"engine":"postgres","version":"16","ephemeralInPreviews":true}}}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("connect git source with database: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	runtime = newFakePreviewDBRuntime()
	rt.execRuntime = func(string) (docker.Runtime, error) { return runtime, nil }
	return rt, db, secret, runtime
}

func TestDeployPreviewEnvironment_ProvisionsEphemeralDatabase(t *testing.T) {
	rt, db, secret, _ := setUpPreviewAppWithDatabase(t)

	rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	preview, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
	if err != nil {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
	}

	wantDBName := "web-pr-42-db-main"
	desired, err := db.GetDesiredDatabase(context.Background(), wantDBName)
	if err != nil {
		t.Fatalf("GetDesiredDatabase(%q) error = %v", wantDBName, err)
	}
	if desired.Engine != "postgres" || desired.Version != "16" {
		t.Errorf("desired database = %+v, want engine=postgres version=16", desired)
	}

	tracked, err := db.GetPreviewEphemeralDatabaseByPreviewAndKey(context.Background(), preview.ID, "main")
	if err != nil {
		t.Fatalf("GetPreviewEphemeralDatabaseByPreviewAndKey() error = %v", err)
	}
	if tracked.DatabaseName != wantDBName || tracked.Status != store.PreviewEphemeralDatabaseStatusProvisioned {
		t.Errorf("tracked = %+v, want database_name=%q status=provisioned", tracked, wantDBName)
	}

	svc, err := db.GetDesiredService(context.Background(), "web-pr-42")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.DatabaseAttachment == nil || svc.DatabaseAttachment.DatabaseName != wantDBName {
		t.Errorf("DatabaseAttachment = %+v, want it to point at %q", svc.DatabaseAttachment, wantDBName)
	}
	if svc.RestartNonce == "" {
		t.Error("RestartNonce is empty, want the attachment to have triggered a restart so a container picks it up")
	}
}

func TestDeployPreviewEnvironment_Synchronize_EphemeralDatabaseIsIdempotent(t *testing.T) {
	rt, db, secret, _ := setUpPreviewAppWithDatabase(t)

	sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main"))
	preview, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
	if err != nil {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
	}
	first, err := db.GetPreviewEphemeralDatabaseByPreviewAndKey(context.Background(), preview.ID, "main")
	if err != nil {
		t.Fatalf("first GetPreviewEphemeralDatabaseByPreviewAndKey() error = %v", err)
	}

	rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("synchronize", 42, "sha2", "main"))
	if rec.Code != http.StatusOK {
		t.Fatalf("synchronize status = %d, body = %s", rec.Code, rec.Body.String())
	}

	second, err := db.GetPreviewEphemeralDatabaseByPreviewAndKey(context.Background(), preview.ID, "main")
	if err != nil {
		t.Fatalf("second GetPreviewEphemeralDatabaseByPreviewAndKey() error = %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("ID changed across synchronize: first = %q, second = %q, want the same tracking row reused", first.ID, second.ID)
	}
}

func TestTeardownPreviewEphemeralDatabase_RemovesContainerAndRows(t *testing.T) {
	rt, db, secret, runtime := setUpPreviewAppWithDatabase(t)

	sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main"))
	preview, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
	if err != nil {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
	}
	tracked, err := db.GetPreviewEphemeralDatabaseByPreviewAndKey(context.Background(), preview.ID, "main")
	if err != nil {
		t.Fatalf("GetPreviewEphemeralDatabaseByPreviewAndKey() error = %v", err)
	}
	runtime.seed(database.ContainerName(tracked.DatabaseName), true)

	rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("closed", 42, "sha1", "main"))
	if rec.Code != http.StatusOK {
		t.Fatalf("closed status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if _, err := db.GetDesiredDatabase(context.Background(), tracked.DatabaseName); !errors.Is(err, store.ErrDatabaseNotFound) {
		t.Errorf("GetDesiredDatabase() error = %v, want ErrDatabaseNotFound", err)
	}
	if _, err := db.GetPreviewEphemeralDatabaseByPreviewAndKey(context.Background(), preview.ID, "main"); !errors.Is(err, store.ErrPreviewEphemeralDatabaseNotFound) {
		t.Errorf("GetPreviewEphemeralDatabaseByPreviewAndKey() error = %v, want ErrPreviewEphemeralDatabaseNotFound", err)
	}
	if _, ok := runtime.containers[database.ContainerName(tracked.DatabaseName)]; ok {
		t.Errorf("container %q still present after teardown", tracked.DatabaseName)
	}
	if _, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42); !errors.Is(err, store.ErrPreviewEnvironmentNotFound) {
		t.Errorf("preview environment row error = %v, want ErrPreviewEnvironmentNotFound (a clean teardown deletes it)", err)
	}
}

// TestTeardownPreviewEphemeralDatabase_HalfSucceeded_ThenRetrySucceeds is
// this feature's own half-succeeded reconciler test (CLAUDE.md section
// 7): a container removal failure (Docker temporarily unreachable, mid
// teardown) must leave the tracking row and the desired database intact
// for a retry, not lose track of either, and a subsequent retry with the
// same underlying failure cleared must fully converge.
func TestTeardownPreviewEphemeralDatabase_HalfSucceeded_ThenRetrySucceeds(t *testing.T) {
	rt, db, secret, runtime := setUpPreviewAppWithDatabase(t)

	sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main"))
	preview, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
	if err != nil {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
	}
	tracked, err := db.GetPreviewEphemeralDatabaseByPreviewAndKey(context.Background(), preview.ID, "main")
	if err != nil {
		t.Fatalf("GetPreviewEphemeralDatabaseByPreviewAndKey() error = %v", err)
	}
	runtime.seed(database.ContainerName(tracked.DatabaseName), true)
	runtime.removeErr = errors.New("engine temporarily unavailable")

	rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("closed", 42, "sha1", "main"))
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("closed (failing) status = %d, want %d, body = %s", rec.Code, http.StatusMultiStatus, rec.Body.String())
	}

	failedTracked, err := db.GetPreviewEphemeralDatabaseByPreviewAndKey(context.Background(), preview.ID, "main")
	if err != nil {
		t.Fatalf("tracking row missing after failed teardown: %v", err)
	}
	if failedTracked.Status != store.PreviewEphemeralDatabaseStatusTeardownFailed || failedTracked.StatusReason == "" {
		t.Errorf("tracked = %+v, want status=teardown_failed with a reason", failedTracked)
	}
	if _, err := db.GetDesiredDatabase(context.Background(), tracked.DatabaseName); err != nil {
		t.Errorf("GetDesiredDatabase() error = %v, want the desired database to still exist for a retry", err)
	}
	if _, ok := runtime.containers[database.ContainerName(tracked.DatabaseName)]; !ok {
		t.Error("container removed despite Remove failing: want it left in place for a retry")
	}
	failedPreview, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
	if err != nil {
		t.Fatalf("preview environment row missing after failed teardown: %v", err)
	}
	if failedPreview.Status != store.PreviewStatusFailed {
		t.Errorf("preview.Status = %q, want %q", failedPreview.Status, store.PreviewStatusFailed)
	}

	runtime.removeErr = nil
	rec = sendPullRequestWebhook(rt, secret, githubPullRequestBody("closed", 42, "sha1", "main"))
	if rec.Code != http.StatusOK {
		t.Fatalf("retried closed status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if _, err := db.GetDesiredDatabase(context.Background(), tracked.DatabaseName); !errors.Is(err, store.ErrDatabaseNotFound) {
		t.Errorf("GetDesiredDatabase() after retry error = %v, want ErrDatabaseNotFound", err)
	}
	if _, err := db.GetPreviewEphemeralDatabaseByPreviewAndKey(context.Background(), preview.ID, "main"); !errors.Is(err, store.ErrPreviewEphemeralDatabaseNotFound) {
		t.Errorf("GetPreviewEphemeralDatabaseByPreviewAndKey() after retry error = %v, want ErrPreviewEphemeralDatabaseNotFound", err)
	}
}

func setUpSeedFromPreview(t *testing.T, seedFrom string, withBackup bool) (*Router, *store.DB, string, *fakeCloneRestoreRunner) {
	t.Helper()
	rt, db, secret, _ := setUpPreviewApp(t)
	ctx := context.Background()
	cookie := loginTestSession(t, rt, db)
	body := `{"repo_url":"https://github.com/org/web.git","branch":"main","databases":{"main":{"engine":"postgres","version":"16","ephemeralInPreviews":true,"seedFrom":"` + seedFrom + `"}}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("connect git source: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed parent: %v", err)
	}
	if err := db.SaveServiceDEK(ctx, "main", []byte("dek")); err != nil {
		t.Fatalf("seed parent dek: %v", err)
	}
	if err := db.SaveSecretValue(ctx, "main", "postgres_password", []byte("parent-secret")); err != nil {
		t.Fatalf("seed parent secret: %v", err)
	}
	if withBackup {
		seedSucceededBackupForCloneRestore(t, db, "main")
	}
	runner := newFakeCloneRestoreRunner()
	rt.cloneRestoreRunner = runner
	rt.execRuntime = func(string) (docker.Runtime, error) { return newFakePreviewDBRuntime(), nil }
	return rt, db, secret, runner
}

func previewReason(t *testing.T, db *store.DB) string {
	t.Helper()
	p, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
	if err != nil {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
	}
	return p.StatusReason
}

func TestSeedFromLatest_RestoresParentBackupWithoutCopyingSecrets(t *testing.T) {
	rt, db, secret, runner := setUpSeedFromPreview(t, "latest", true)
	rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	call := runner.awaitCall(t)
	if call.sourceDatabaseName != "main" || call.newDatabaseName != "web-pr-42-db-main" || call.backupHistoryID != "bkh_1" || call.engine != store.EnginePostgres {
		t.Errorf("restore call = %+v", call)
	}
	if has, _ := db.HasSecretValue(context.Background(), "web-pr-42-db-main", "postgres_password"); has {
		t.Error("preview database carries a copied secret, want it generated at provision")
	}
	if got := previewReason(t, db); !strings.Contains(got, "seeding web-pr-42-db-main from the latest backup of main") {
		t.Errorf("status reason = %q", got)
	}

	// A synchronize push must not restore over a database that already has data.
	sendPullRequestWebhook(rt, secret, githubPullRequestBody("synchronize", 42, "sha2", "main"))
	select {
	case c := <-runner.calls:
		t.Errorf("redeploy restored again: %+v", c)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestSeedFromLatest_NoBackupFallsBackToEmpty(t *testing.T) {
	rt, db, secret, runner := setUpSeedFromPreview(t, "latest", false)
	sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main"))
	if _, err := db.GetDesiredDatabase(context.Background(), "web-pr-42-db-main"); err != nil {
		t.Fatalf("preview database missing: %v", err)
	}
	if got := previewReason(t, db); !strings.Contains(got, "no succeeded backup of main exists, the preview database is empty") {
		t.Errorf("status reason = %q", got)
	}
	select {
	case c := <-runner.calls:
		t.Errorf("restore started without a backup: %+v", c)
	default:
	}
}

func TestSeedFromNone_StartsEmpty(t *testing.T) {
	rt, db, secret, runner := setUpSeedFromPreview(t, "none", true)
	sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main"))
	select {
	case c := <-runner.calls:
		t.Errorf("seedFrom none restored: %+v", c)
	case <-time.After(200 * time.Millisecond):
	}
	if got := previewReason(t, db); strings.Contains(got, "seed") {
		t.Errorf("status reason = %q, want no seed note", got)
	}
}

func TestSeedFromLatest_LowDiskFallsBackToEmpty(t *testing.T) {
	t.Setenv("APP_DATA_DIR", t.TempDir())
	t.Setenv("APP_MIN_BACKUP_DISK_MB", "999999999")
	rt, db, secret, runner := setUpSeedFromPreview(t, "latest", true)
	sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main"))
	select {
	case c := <-runner.calls:
		t.Errorf("restore started on a full disk: %+v", c)
	case <-time.After(200 * time.Millisecond):
	}
	if got := previewReason(t, db); !strings.Contains(got, "not enough free disk") {
		t.Errorf("status reason = %q", got)
	}
}
