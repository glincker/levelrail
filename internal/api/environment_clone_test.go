package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestEnvironmentCloneRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/environments/env_src/clone/preview?new_environment_name=staging-eu"},
		{http.MethodPost, "/api/v1/environments/env_src/clone"},
	})
}

// seedCloneFixture saves a project, a source environment (env_src) under
// it, and two apps tagged with it: "web" carries almost every field a
// clone needs to prove it copies (env vars, resources, health, a
// volume, a domain, a declared secret env var), "worker" is a plain
// second app so a clone-the-whole-environment test has more than one
// app to prove it doesn't stop after the first. Also seeds one
// scheduled task on "web" and the source environment's own shared env
// vars (one plain, one secret-marked).
func seedCloneFixture(t *testing.T, db *store.DB) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-09-01T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := db.SaveEnvironment(ctx, store.Environment{ID: "env_src", ProjectID: "proj_1", Name: "staging", CreatedAt: "2026-09-01T00:00:00Z"}); err != nil {
		t.Fatalf("seed environment: %v", err)
	}

	readyTimeout := 30 * time.Second
	web := store.DesiredService{
		Name:        "web",
		Image:       "levelrail/web:7",
		Port:        3000,
		HostPort:    intPtr(8080),
		BindAddress: "private",
		Domains:     []string{"web-staging.example.com"},
		Env:         map[string]string{"LOG_LEVEL": "debug"},
		SecretEnv:   []store.SecretEnvRef{{Name: "API_KEY", Required: true}},
		Resources:   &store.ServiceResources{MemoryBytes: 256 << 20},
		Health: &store.ServiceHealth{
			Readiness:    &store.ServiceProbe{Path: "/healthz", Interval: 10 * time.Second},
			ReadyTimeout: readyTimeout,
		},
		Volumes:  []store.ServiceVolume{{Name: "app-web-data", ContainerPath: "/data"}},
		Labels:   map[string]string{"team": "platform"},
		Strategy: "blue-green",
		Replicas: 2,
	}
	if err := db.SaveDesiredService(ctx, web); err != nil {
		t.Fatalf("seed web: %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "web", "proj_1"); err != nil {
		t.Fatalf("project web: %v", err)
	}
	if err := db.SetServiceEnvironment(ctx, "web", "env_src"); err != nil {
		t.Fatalf("tag web: %v", err)
	}
	if err := db.SaveScheduledTask(ctx, store.ScheduledTask{
		ID: "sct_1", ServiceName: "web", Command: []string{"./cleanup"}, Schedule: "@daily",
		Enabled: true, ConcurrencyPolicy: store.ScheduledTaskConcurrencyForbid,
	}); err != nil {
		t.Fatalf("seed scheduled task: %v", err)
	}

	worker := store.DesiredService{Name: "worker", Image: "levelrail/worker:3", Port: 4000}
	if err := db.SaveDesiredService(ctx, worker); err != nil {
		t.Fatalf("seed worker: %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "worker", "proj_1"); err != nil {
		t.Fatalf("project worker: %v", err)
	}
	if err := db.SetServiceEnvironment(ctx, "worker", "env_src"); err != nil {
		t.Fatalf("tag worker: %v", err)
	}

	if err := db.SetEnvironmentEnvVars(ctx, "env_src", map[string]string{"REGION": "us-east"}); err != nil {
		t.Fatalf("seed shared env var: %v", err)
	}
	if err := db.SetEnvironmentSecretEnvVar(ctx, "env_src", "SHARED_TOKEN"); err != nil {
		t.Fatalf("seed shared secret env var: %v", err)
	}
}

func TestHandleEnvironmentClonePreview(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedCloneFixture(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/environments/env_src/clone/preview?new_environment_name=staging-eu", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got environmentClonePreviewResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SourceEnvironment.Name != "staging" {
		t.Errorf("SourceEnvironment.Name = %q, want staging", got.SourceEnvironment.Name)
	}
	if len(got.Apps) != 2 {
		t.Fatalf("len(Apps) = %d, want 2; got %+v", len(got.Apps), got.Apps)
	}

	byName := map[string]environmentCloneAppPreview{}
	for _, a := range got.Apps {
		byName[a.SourceApp] = a
	}
	web, ok := byName["web"]
	if !ok {
		t.Fatalf("web not present in preview: %+v", got.Apps)
	}
	if web.SuggestedNewName != "web-staging-eu" {
		t.Errorf("web SuggestedNewName = %q, want web-staging-eu", web.SuggestedNewName)
	}
	if len(web.CurrentDomains) != 1 || web.CurrentDomains[0] != "web-staging.example.com" {
		t.Errorf("web CurrentDomains = %v, want [web-staging.example.com]", web.CurrentDomains)
	}
	if len(web.SecretEnvKeys) != 1 || web.SecretEnvKeys[0] != "API_KEY" {
		t.Errorf("web SecretEnvKeys = %v, want [API_KEY]", web.SecretEnvKeys)
	}
	if !web.HasHealthCheck || !web.HasHostPortPin || web.VolumeCount != 1 {
		t.Errorf("web preview missing expected flags: %+v", web)
	}
	if web.ScheduledTaskCount != 1 {
		t.Errorf("web ScheduledTaskCount = %d, want 1", web.ScheduledTaskCount)
	}
	if len(got.EnvironmentEnvVarKeys) != 1 || got.EnvironmentEnvVarKeys[0] != "REGION" {
		t.Errorf("EnvironmentEnvVarKeys = %v, want [REGION]", got.EnvironmentEnvVarKeys)
	}
	if len(got.EnvironmentSecretEnvKeys) != 1 || got.EnvironmentSecretEnvKeys[0] != "SHARED_TOKEN" {
		t.Errorf("EnvironmentSecretEnvKeys = %v, want [SHARED_TOKEN]", got.EnvironmentSecretEnvKeys)
	}
	if len(got.UnclonedFields) == 0 || got.Note == "" {
		t.Error("expected UnclonedFields and Note to be populated")
	}

	// A preview must never create anything.
	envs, err := db.ListEnvironmentsByProject(context.Background(), "proj_1")
	if err != nil {
		t.Fatalf("ListEnvironmentsByProject: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("preview created an environment: %+v", envs)
	}
}

func TestHandleEnvironmentClonePreview_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/environments/env_missing/clone/preview?new_environment_name=x", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleEnvironmentClone_HappyPath(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedCloneFixture(t, db)
	ctx := context.Background()

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/environments/env_src/clone", `{"new_environment_name":"staging-eu"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got environmentCloneResultResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Environment.Name != "staging-eu" || got.Environment.ProjectID != "proj_1" {
		t.Fatalf("Environment = %+v, want name staging-eu in proj_1", got.Environment)
	}
	if got.Environment.ID == "env_src" {
		t.Fatal("new environment must have its own id")
	}
	if len(got.Apps) != 2 {
		t.Fatalf("len(Apps) = %d, want 2", len(got.Apps))
	}
	if got.CopiedSecretValues {
		t.Error("CopiedSecretValues = true, want false (not requested)")
	}

	// The cloned "web" app: image/env/resources/health copied, name and
	// volumes regenerated, host port and domains cleared.
	cloned, err := db.GetDesiredService(ctx, "web-staging-eu")
	if err != nil {
		t.Fatalf("GetDesiredService(web-staging-eu): %v", err)
	}
	if cloned.Image != "levelrail/web:7" {
		t.Errorf("cloned Image = %q, want levelrail/web:7", cloned.Image)
	}
	if cloned.Env["LOG_LEVEL"] != "debug" {
		t.Errorf("cloned Env = %v, want LOG_LEVEL=debug", cloned.Env)
	}
	if cloned.HostPort != nil {
		t.Errorf("cloned HostPort = %v, want nil (cleared)", cloned.HostPort)
	}
	if len(cloned.Domains) != 0 {
		t.Errorf("cloned Domains = %v, want none (not copied by default)", cloned.Domains)
	}
	if cloned.Resources == nil || cloned.Resources.MemoryBytes != 256<<20 {
		t.Errorf("cloned Resources = %+v, want MemoryBytes 256MiB", cloned.Resources)
	}
	if cloned.Health == nil || cloned.Health.Readiness == nil || cloned.Health.Readiness.Path != "/healthz" {
		t.Errorf("cloned Health = %+v, want readiness path /healthz", cloned.Health)
	}
	if len(cloned.Volumes) != 1 || cloned.Volumes[0].Name != "app-web-staging-eu-data" {
		t.Errorf("cloned Volumes = %+v, want one volume named app-web-staging-eu-data", cloned.Volumes)
	}
	if len(cloned.SecretEnv) != 1 || cloned.SecretEnv[0].Name != "API_KEY" || !cloned.SecretEnv[0].Required {
		t.Errorf("cloned SecretEnv = %+v, want [{API_KEY true}]", cloned.SecretEnv)
	}
	if cloned.ProjectID != "proj_1" {
		t.Errorf("cloned ProjectID = %q, want proj_1", cloned.ProjectID)
	}
	if cloned.EnvironmentID != got.Environment.ID {
		t.Errorf("cloned EnvironmentID = %q, want %q", cloned.EnvironmentID, got.Environment.ID)
	}
	if cloned.Replicas != 2 || cloned.Strategy != "blue-green" {
		t.Errorf("cloned Strategy/Replicas = %s/%d, want blue-green/2", cloned.Strategy, cloned.Replicas)
	}

	// The cloned "worker" app exists too: the whole environment was
	// cloned, not just its first app.
	if _, err := db.GetDesiredService(ctx, "worker-staging-eu"); err != nil {
		t.Fatalf("GetDesiredService(worker-staging-eu): %v", err)
	}

	// The scheduled task followed "web" to its clone, under a fresh ID.
	tasks, err := db.ListScheduledTasksForService(ctx, "web-staging-eu")
	if err != nil {
		t.Fatalf("ListScheduledTasksForService: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Schedule != "@daily" || tasks[0].ID == "sct_1" {
		t.Errorf("cloned scheduled tasks = %+v, want one fresh copy of sct_1", tasks)
	}

	// The environment's own shared env vars: plain value copied, secret
	// key declared but with no value set (copy_secret_values wasn't
	// requested).
	sharedVars, err := db.ListEnvironmentEnvVars(ctx, got.Environment.ID)
	if err != nil {
		t.Fatalf("ListEnvironmentEnvVars: %v", err)
	}
	if sharedVars["REGION"] != "us-east" {
		t.Errorf("cloned shared env vars = %v, want REGION=us-east", sharedVars)
	}
	sharedSecretKeys, err := db.ListEnvironmentSecretEnvKeys(ctx, got.Environment.ID)
	if err != nil {
		t.Fatalf("ListEnvironmentSecretEnvKeys: %v", err)
	}
	if len(sharedSecretKeys) != 1 || sharedSecretKeys[0] != "SHARED_TOKEN" {
		t.Errorf("cloned shared secret keys = %v, want [SHARED_TOKEN]", sharedSecretKeys)
	}

	// A deploy attempt was recorded for the new app, sourced as "clone".
	attempts, err := db.ListDeployAttempts(ctx, "web-staging-eu")
	if err != nil {
		t.Fatalf("ListDeployAttempts: %v", err)
	}
	if len(attempts) != 1 || attempts[0].Source != store.DeployAttemptSourceClone {
		t.Errorf("deploy attempts = %+v, want one with source %q", attempts, store.DeployAttemptSourceClone)
	}
}

func TestHandleEnvironmentClone_NameConflict(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedCloneFixture(t, db)
	ctx := context.Background()
	// Pre-occupy the auto-suggested name for "web".
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web-staging-eu", Image: "other:1", Port: 80}); err != nil {
		t.Fatalf("seed conflicting app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/environments/env_src/clone", `{"new_environment_name":"staging-eu"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}

	// Nothing else got created: the whole request is rejected up front.
	if _, err := db.GetDesiredService(ctx, "worker-staging-eu"); err == nil {
		t.Error("worker-staging-eu should not exist: name conflicts are caught before any write")
	}
}

func TestHandleEnvironmentClone_AppRenameAndDomainOverride(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedCloneFixture(t, db)
	ctx := context.Background()

	body := `{"new_environment_name":"staging-eu","apps":[{"source_app":"web","new_name":"web-eu","domains":["web-eu.example.com"]}]}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/environments/env_src/clone", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	cloned, err := db.GetDesiredService(ctx, "web-eu")
	if err != nil {
		t.Fatalf("GetDesiredService(web-eu): %v", err)
	}
	if len(cloned.Domains) != 1 || cloned.Domains[0] != "web-eu.example.com" {
		t.Errorf("cloned Domains = %v, want [web-eu.example.com]", cloned.Domains)
	}
	if len(cloned.Volumes) != 1 || cloned.Volumes[0].Name != "app-web-eu-data" {
		t.Errorf("cloned Volumes = %+v, want app-web-eu-data", cloned.Volumes)
	}
}

func TestHandleEnvironmentClone_DomainAlreadyTaken(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedCloneFixture(t, db)

	// The source app itself still owns web-staging.example.com; asking
	// the clone to reuse it must fail with the same *store.ErrDomainTaken
	// SaveDesiredService itself returns, not a generic 500.
	body := `{"new_environment_name":"staging-eu","apps":[{"source_app":"web","domains":["web-staging.example.com"]}]}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/environments/env_src/clone", body))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleEnvironmentClone_NoAppsTagged(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-09-01T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := db.SaveEnvironment(ctx, store.Environment{ID: "env_empty", ProjectID: "proj_1", Name: "empty", CreatedAt: "2026-09-01T00:00:00Z"}); err != nil {
		t.Fatalf("seed environment: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/environments/env_empty/clone", `{"new_environment_name":"x"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleEnvironmentClone_MissingNewName(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedCloneFixture(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/environments/env_src/clone", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleEnvironmentClone_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/environments/env_missing/clone", `{"new_environment_name":"x"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleEnvironmentClone_CopySecretValues_RealSecretsManager proves
// copy_secret_values: true actually round-trips real plaintext, through
// the real internal/secrets.Manager (envelope encryption), not a fake:
// the same proof-of-real-encryption shape
// TestHandleUpdateEmailSettings_RealSecretsManager_RoundTripsThroughEncryption
// already establishes elsewhere in this package. Without the flag, a
// sibling test above already proves no value crosses at all.
func TestHandleEnvironmentClone_CopySecretValues_RealSecretsManager(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey: %v", err)
	}
	manager := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithSecretSetter(manager))
	cookie := loginTestSession(t, rt, db)
	seedCloneFixture(t, db)
	ctx := context.Background()

	if err := manager.SetValueGuarded(ctx, "web", "API_KEY", "sk-source-value", false); err != nil {
		t.Fatalf("seed source app secret: %v", err)
	}
	if err := manager.SetValueGuarded(ctx, store.EnvironmentEnvSecretsKey("env_src"), "SHARED_TOKEN", "shared-source-value", false); err != nil {
		t.Fatalf("seed source shared secret: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/environments/env_src/clone", `{"new_environment_name":"staging-eu","copy_secret_values":true}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got environmentCloneResultResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.CopiedSecretValues {
		t.Error("CopiedSecretValues = false, want true")
	}

	value, err := manager.Resolve(ctx, "web-staging-eu", "API_KEY")
	if err != nil {
		t.Fatalf("Resolve cloned app secret: %v", err)
	}
	if value != "sk-source-value" {
		t.Errorf("cloned app secret = %q, want sk-source-value", value)
	}

	sharedValue, err := manager.Resolve(ctx, store.EnvironmentEnvSecretsKey(got.Environment.ID), "SHARED_TOKEN")
	if err != nil {
		t.Fatalf("Resolve cloned shared secret: %v", err)
	}
	if sharedValue != "shared-source-value" {
		t.Errorf("cloned shared secret = %q, want shared-source-value", sharedValue)
	}

	// The source app's own secret must be untouched: this is a copy, not
	// a move.
	sourceValue, err := manager.Resolve(ctx, "web", "API_KEY")
	if err != nil || sourceValue != "sk-source-value" {
		t.Errorf("source secret changed: value=%q err=%v", sourceValue, err)
	}
}
