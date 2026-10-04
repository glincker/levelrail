package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeIsolationRuntime is a hand-written fake docker.Runtime recording
// every Exec call's command, the same "every method a compile-satisfying
// stub except what the feature under test actually calls" convention
// fakePreviewDBRuntime (preview_environments_databases_test.go) already
// establishes for this interface.
type fakeIsolationRuntime struct {
	containers map[string]*docker.ContainerState
	execCalls  []string
	execErr    error
}

func newFakeIsolationRuntime() *fakeIsolationRuntime {
	return &fakeIsolationRuntime{containers: map[string]*docker.ContainerState{}}
}

func (f *fakeIsolationRuntime) seed(name string, running bool) {
	f.containers[name] = &docker.ContainerState{ID: "c-" + name, Name: name, Running: running}
}

func (f *fakeIsolationRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	cs, ok := f.containers[name]
	if !ok {
		return nil, nil
	}
	cp := *cs
	return &cp, nil
}

func (f *fakeIsolationRuntime) Exec(_ context.Context, _ string, cmd []string) (io.ReadCloser, error) {
	if len(cmd) > 0 {
		// The full command, not just its last argument: psql's whole SQL
		// script lives in one shell argument (still findable via
		// strings.Contains at the end), but redis-cli's ACL tokens are
		// spread across several separate argv entries.
		f.execCalls = append(f.execCalls, strings.Join(cmd, " "))
	}
	if f.execErr != nil {
		return nil, f.execErr
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (f *fakeIsolationRuntime) ExecWithInput(ctx context.Context, id string, cmd []string, _ io.Reader) (io.ReadCloser, error) {
	return f.Exec(ctx, id, cmd)
}
func (f *fakeIsolationRuntime) Stop(context.Context, string, time.Duration) error { return nil }
func (f *fakeIsolationRuntime) Remove(context.Context, string, bool) error        { return nil }
func (f *fakeIsolationRuntime) Create(context.Context, docker.ContainerSpec) (string, error) {
	return "", nil
}
func (f *fakeIsolationRuntime) Start(context.Context, string) error { return nil }
func (f *fakeIsolationRuntime) Events(context.Context) (<-chan docker.Event, <-chan error) {
	return nil, nil
}
func (f *fakeIsolationRuntime) ListImages(context.Context, string) ([]docker.ImageInfo, error) {
	return nil, nil
}
func (f *fakeIsolationRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return nil, nil
}
func (f *fakeIsolationRuntime) UpdateResources(context.Context, string, docker.Resources) error {
	return nil
}
func (f *fakeIsolationRuntime) EnsureVolume(context.Context, string) error { return nil }
func (f *fakeIsolationRuntime) EnsureNetwork(context.Context, string) (string, error) {
	return "", nil
}
func (f *fakeIsolationRuntime) RemoveNetwork(context.Context, string) error          { return nil }
func (f *fakeIsolationRuntime) NetworkConnect(context.Context, string, string) error { return nil }
func (f *fakeIsolationRuntime) NetworkDisconnect(context.Context, string, string, bool) error {
	return nil
}
func (f *fakeIsolationRuntime) ListNetworksByPrefix(context.Context, string) ([]docker.NetworkInfo, error) {
	return nil, nil
}

// setUpPreviewAppWithDatabaseIsolationEngine seeds app "web" with a git
// source declaring an isolatedInPreviews database on engine that points
// at an existing managed database "main" (not one the preview itself
// owns), with a real secrets.Manager and a fake runtime whose container
// for "main" is already running. Parameterized on engine so the same
// fixture covers both the Postgres role path and the Redis ACL user
// path.
func setUpPreviewAppWithDatabaseIsolationEngine(t *testing.T, engine string) (rt *Router, db *store.DB, secret string, runtime *fakeIsolationRuntime) {
	t.Helper()
	db = openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	manager := secrets.NewManager(db, mk)
	gitSourceSecrets := newFakeGitSourceSecrets()
	rt = NewRouter(discardLogger(), testBrand(), db, WithGitSourceSecrets(gitSourceSecrets), WithSecretSetter(manager))
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: engine, Version: "16"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}

	body := fmt.Sprintf(`{"repo_url":"https://github.com/org/web.git","branch":"main","databases":{"main":{"engine":%q,"version":"16","isolatedInPreviews":true}}}`, engine)
	created := connectGitSource(t, rt, cookie, body)
	setPreviewEnabled(t, rt, cookie)

	rt.builder = &sequencedBuilder{db: db, tag: "levelrail/web-pr-42:sha1"}
	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), nil)

	runtime = newFakeIsolationRuntime()
	runtime.seed(databaseContainerName("main"), true)
	rt.execRuntime = func(string) (docker.Runtime, error) { return runtime, nil }

	return rt, db, created.WebhookSecret, runtime
}

// The tests below cover both the Postgres role path and the Redis ACL
// path, table-driven against the same fixture and assertions wherever
// the two engines' behavior is identical (provision, idempotency,
// teardown, half-succeeded teardown), so a regression in either
// engine's exec dispatch shows up as a failure on its own row.

func TestDeployPreviewEnvironment_Synchronize_DatabaseIsolationIsIdempotent(t *testing.T) {
	for _, engine := range []string{store.EnginePostgres, store.EngineRedis} {
		t.Run(engine, func(t *testing.T) {
			rt, db, secret, runtime := setUpPreviewAppWithDatabaseIsolationEngine(t, engine)

			sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main"))
			preview, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
			if err != nil {
				t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
			}
			first, err := db.GetPreviewDatabaseIsolationByPreviewAndKey(context.Background(), preview.ID, "main")
			if err != nil {
				t.Fatalf("first GetPreviewDatabaseIsolationByPreviewAndKey() error = %v", err)
			}
			execCallsAfterFirst := len(runtime.execCalls)

			rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("synchronize", 42, "sha2", "main"))
			if rec.Code != http.StatusOK {
				t.Fatalf("synchronize status = %d, body = %s", rec.Code, rec.Body.String())
			}

			second, err := db.GetPreviewDatabaseIsolationByPreviewAndKey(context.Background(), preview.ID, "main")
			if err != nil {
				t.Fatalf("second GetPreviewDatabaseIsolationByPreviewAndKey() error = %v", err)
			}
			if second.ID != first.ID {
				t.Errorf("ID changed across synchronize: first = %q, second = %q, want the same tracking row reused", first.ID, second.ID)
			}
			if len(runtime.execCalls) != execCallsAfterFirst {
				t.Errorf("execCalls after synchronize = %d, want %d (no re-provision)", len(runtime.execCalls), execCallsAfterFirst)
			}
		})
	}
}

func TestTeardownPreviewDatabaseIsolation_DropsRoleAndRows(t *testing.T) {
	cases := []struct {
		engine      string
		wantExecSub string
	}{
		{store.EnginePostgres, "DROP ROLE"},
		{store.EngineRedis, "ACL DELUSER"},
	}
	for _, tc := range cases {
		t.Run(tc.engine, func(t *testing.T) {
			rt, db, secret, runtime := setUpPreviewAppWithDatabaseIsolationEngine(t, tc.engine)

			sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main"))
			preview, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
			if err != nil {
				t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
			}
			if _, err := db.GetPreviewDatabaseIsolationByPreviewAndKey(context.Background(), preview.ID, "main"); err != nil {
				t.Fatalf("GetPreviewDatabaseIsolationByPreviewAndKey() error = %v", err)
			}

			rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("closed", 42, "sha1", "main"))
			if rec.Code != http.StatusOK {
				t.Fatalf("closed status = %d, body = %s", rec.Code, rec.Body.String())
			}

			if _, err := db.GetPreviewDatabaseIsolationByPreviewAndKey(context.Background(), preview.ID, "main"); !errors.Is(err, store.ErrPreviewDatabaseIsolationNotFound) {
				t.Errorf("GetPreviewDatabaseIsolationByPreviewAndKey() error = %v, want ErrPreviewDatabaseIsolationNotFound", err)
			}
			// The database itself must survive teardown: only the role/user is dropped.
			if _, err := db.GetDesiredDatabase(context.Background(), "main"); err != nil {
				t.Errorf("GetDesiredDatabase(main) error = %v, want it to still exist after teardown", err)
			}
			if len(runtime.execCalls) != 2 {
				t.Fatalf("execCalls = %d, want 2 (create then drop)", len(runtime.execCalls))
			}
			if !strings.Contains(runtime.execCalls[1], tc.wantExecSub) {
				t.Errorf("teardown exec call = %q, want it to contain %q", runtime.execCalls[1], tc.wantExecSub)
			}
		})
	}
}

// TestTeardownPreviewDatabaseIsolation_HalfSucceeded_ThenRetrySucceeds is
// this feature's own half-succeeded reconciler test (CLAUDE.md section
// 7): an exec failure while dropping the role/user must leave the
// tracking row intact for a retry, not lose track of it, and a
// subsequent retry with the failure cleared must fully converge. Runs
// against both engines since each dispatches teardown differently.
func TestTeardownPreviewDatabaseIsolation_HalfSucceeded_ThenRetrySucceeds(t *testing.T) {
	for _, engine := range []string{store.EnginePostgres, store.EngineRedis} {
		t.Run(engine, func(t *testing.T) {
			rt, db, secret, runtime := setUpPreviewAppWithDatabaseIsolationEngine(t, engine)

			sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main"))
			preview, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
			if err != nil {
				t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
			}
			if _, err := db.GetPreviewDatabaseIsolationByPreviewAndKey(context.Background(), preview.ID, "main"); err != nil {
				t.Fatalf("GetPreviewDatabaseIsolationByPreviewAndKey() error = %v", err)
			}

			runtime.execErr = errors.New("engine temporarily unavailable")
			rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("closed", 42, "sha1", "main"))
			if rec.Code != http.StatusMultiStatus {
				t.Fatalf("closed (failing) status = %d, want %d, body = %s", rec.Code, http.StatusMultiStatus, rec.Body.String())
			}

			failedTracked, err := db.GetPreviewDatabaseIsolationByPreviewAndKey(context.Background(), preview.ID, "main")
			if err != nil {
				t.Fatalf("tracking row missing after failed teardown: %v", err)
			}
			if failedTracked.Status != store.PreviewDatabaseIsolationStatusTeardownFailed || failedTracked.StatusReason == "" {
				t.Errorf("tracked = %+v, want status=teardown_failed with a reason", failedTracked)
			}

			runtime.execErr = nil
			rec = sendPullRequestWebhook(rt, secret, githubPullRequestBody("closed", 42, "sha1", "main"))
			if rec.Code != http.StatusOK {
				t.Fatalf("retried closed status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if _, err := db.GetPreviewDatabaseIsolationByPreviewAndKey(context.Background(), preview.ID, "main"); !errors.Is(err, store.ErrPreviewDatabaseIsolationNotFound) {
				t.Errorf("GetPreviewDatabaseIsolationByPreviewAndKey() after retry error = %v, want ErrPreviewDatabaseIsolationNotFound", err)
			}
		})
	}
}

func TestDeployPreviewEnvironment_ProvisionsDatabaseIsolation_ByEngine(t *testing.T) {
	cases := []struct {
		engine      string
		wantExecSub string
	}{
		{store.EnginePostgres, "CREATE ROLE"},
		{store.EngineRedis, "ACL SETUSER"},
	}
	for _, tc := range cases {
		t.Run(tc.engine, func(t *testing.T) {
			rt, db, secret, runtime := setUpPreviewAppWithDatabaseIsolationEngine(t, tc.engine)

			rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main"))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}

			preview, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
			if err != nil {
				t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
			}

			tracked, err := db.GetPreviewDatabaseIsolationByPreviewAndKey(context.Background(), preview.ID, "main")
			if err != nil {
				t.Fatalf("GetPreviewDatabaseIsolationByPreviewAndKey() error = %v", err)
			}
			if tracked.DatabaseName != "main" || tracked.Status != store.PreviewDatabaseIsolationStatusProvisioned {
				t.Errorf("tracked = %+v, want database_name=main status=provisioned", tracked)
			}
			if tracked.RoleName == "" {
				t.Error("RoleName is empty")
			}

			if _, err := db.GetDesiredDatabase(context.Background(), "main"); err != nil {
				t.Errorf("GetDesiredDatabase(main) error = %v, want it to still exist", err)
			}

			if len(runtime.execCalls) != 1 {
				t.Fatalf("execCalls = %d, want 1", len(runtime.execCalls))
			}
			if !strings.Contains(runtime.execCalls[0], tc.wantExecSub) {
				t.Errorf("exec call = %q, want it to contain %q", runtime.execCalls[0], tc.wantExecSub)
			}
		})
	}
}

// TestProvisionOneDatabaseIsolation_UnsupportedEngineRejected checks the
// engine allowlist directly: an engine that is neither Postgres nor
// Redis must fail loudly rather than silently skip or half-provision.
func TestProvisionOneDatabaseIsolation_UnsupportedEngineRejected(t *testing.T) {
	rt, db, secret, _ := setUpPreviewAppWithDatabaseIsolationEngine(t, store.EngineRedis)

	// Switch the underlying database's engine to one isolation doesn't
	// support, after the git source already declared isolatedInPreviews
	// against it: provisioning must reject it on the next deploy rather
	// than treating it as Redis or Postgres by accident.
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EngineMySQL, Version: "8"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}

	rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 43, "sha1", "main"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	preview, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 43)
	if err != nil {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
	}
	if _, err := db.GetPreviewDatabaseIsolationByPreviewAndKey(context.Background(), preview.ID, "main"); !errors.Is(err, store.ErrPreviewDatabaseIsolationNotFound) {
		t.Errorf("GetPreviewDatabaseIsolationByPreviewAndKey() error = %v, want ErrPreviewDatabaseIsolationNotFound (provisioning should have been rejected, not silently half-done)", err)
	}
}
