package deploy

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeBuilder and fakeServiceStore are hand-written fakes, not a mocking
// framework, the same pattern established across every controller's
// tests in this codebase.
type fakeBuilder struct {
	result *build.Result
	err    error

	lastReq  build.Request
	calls    int
	sawEvent bool

	// Railpack-specific fields, kept separate from the Dockerfile fields
	// above so a test can assert exactly one path was exercised (e.g.
	// deployRailpack must never call Build, deployDockerfile must never
	// call BuildRailpack).
	railpackResult  *build.Result
	railpackErr     error
	lastRailpackReq build.RailpackRequest
	railpackCalls   int
}

func (f *fakeBuilder) Build(_ context.Context, req build.Request, progress func(build.ProgressEvent)) (*build.Result, error) {
	f.calls++
	f.lastReq = req
	if progress != nil {
		progress(build.ProgressEvent{Step: "fake", Completed: true})
		f.sawEvent = true
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func (f *fakeBuilder) BuildRailpack(_ context.Context, req build.RailpackRequest, progress func(build.ProgressEvent)) (*build.Result, error) {
	f.railpackCalls++
	f.lastRailpackReq = req
	if progress != nil {
		progress(build.ProgressEvent{Step: "fake-railpack", Completed: true})
		f.sawEvent = true
	}
	if f.railpackErr != nil {
		return nil, f.railpackErr
	}
	return f.railpackResult, nil
}

type fakeServiceStore struct {
	saveErr   error
	saved     store.DesiredService
	saveCalls int

	// savedByName accumulates every SaveDesiredService call, keyed by
	// name, for internal/deploy's own DeploySpec tests (multi_test.go),
	// which fan out N saves in one call; every single-Deploy test in this
	// file only ever checks `saved` (the most recent), unaffected by this
	// addition.
	savedByName map[string]store.DesiredService
	// failOnName, when non-empty, makes SaveDesiredService return saveErr
	// only for that one service name (every other name succeeds),
	// letting a DeploySpec test simulate one service's own save failing
	// while its siblings still succeed. Every existing single-Deploy test
	// in this file leaves this empty, so saveErr keeps applying
	// unconditionally for them, unchanged from before this field existed.
	failOnName string

	// registryCredentials backs GetRegistryCredentialByName for the
	// build.type: image + registryCredential tests; unset means every
	// lookup returns store.ErrRegistryCredentialNotFound.
	registryCredentials map[string]store.RegistryCredential

	// databases backs GetDesiredDatabase for validateEnv's { from: ... }
	// tests; unset means every lookup returns store.ErrDatabaseNotFound,
	// the same "empty means not found" default registryCredentials
	// already establishes.
	databases map[string]store.DesiredDatabase
}

func (f *fakeServiceStore) GetRegistryCredentialByName(_ context.Context, name string) (store.RegistryCredential, error) {
	cred, ok := f.registryCredentials[name]
	if !ok {
		return store.RegistryCredential{}, store.ErrRegistryCredentialNotFound
	}
	return cred, nil
}

func (f *fakeServiceStore) GetDesiredDatabase(_ context.Context, name string) (*store.DesiredDatabase, error) {
	db, ok := f.databases[name]
	if !ok {
		return nil, store.ErrDatabaseNotFound
	}
	return &db, nil
}

func (f *fakeServiceStore) SaveDesiredService(_ context.Context, svc store.DesiredService) error {
	f.saveCalls++
	f.saved = svc
	if f.savedByName == nil {
		f.savedByName = map[string]store.DesiredService{}
	}
	f.savedByName[svc.Name] = svc
	if f.failOnName != "" {
		if svc.Name == f.failOnName {
			return f.saveErr
		}
		return nil
	}
	return f.saveErr
}

// fakeSecretChecker is a hand-written fake for SecretChecker: a set of
// (service, key) pairs considered to exist, same pattern as the other
// fakes in this file.
type fakeSecretChecker struct {
	set         map[string]bool
	err         error
	existsCalls int
	lastService string
	lastEnvKey  string
}

func newFakeSecretChecker(existing ...string) *fakeSecretChecker {
	set := make(map[string]bool, len(existing))
	for _, k := range existing {
		set[k] = true
	}
	return &fakeSecretChecker{set: set}
}

func (f *fakeSecretChecker) Exists(_ context.Context, serviceName, envKey string) (bool, error) {
	f.existsCalls++
	f.lastService = serviceName
	f.lastEnvKey = envKey
	if f.err != nil {
		return false, f.err
	}
	return f.set[serviceName+"/"+envKey], nil
}

// fakeBuildMetricsRecorder is a hand-written fake for BuildMetricsRecorder,
// same pattern as the other fakes in this file.
type fakeBuildMetricsRecorder struct {
	err             error
	calls           int
	lastServiceName string
	lastDuration    time.Duration
}

func (f *fakeBuildMetricsRecorder) RecordBuildDuration(_ context.Context, serviceName string, d time.Duration, _ time.Time) error {
	f.calls++
	f.lastServiceName = serviceName
	f.lastDuration = d
	return f.err
}

func dockerfileService() spec.Service {
	return spec.Service{
		Build: spec.Build{Type: spec.BuildDockerfile, Path: "./Dockerfile"},
		Port:  3000,
	}
}

func imageService() spec.Service {
	return spec.Service{
		Build: spec.Build{Type: spec.BuildImage, Image: "ghcr.io/org/app:v1.2.3"},
		Port:  3000,
	}
}

func railpackService() spec.Service {
	// No build.Path: unlike spec.BuildDockerfile, spec.BuildRailpack has
	// no user-authored file to point at, Railpack's own detection reads
	// SourceDir directly.
	return spec.Service{
		Build: spec.Build{Type: spec.BuildRailpack},
		Port:  3000,
	}
}

func TestPipeline_Deploy_Dockerfile_Success(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}
	svcStore := &fakeServiceStore{}
	p := New(builder, svcStore)

	tag, err := p.Deploy(context.Background(), Request{
		ServiceName: "web",
		Service:     dockerfileService(),
		SourceDir:   "/repo",
		CommitSHA:   "abc1234",
		ImageRepo:   "levelrail/web",
	}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if tag != "levelrail/web:abc1234" {
		t.Errorf("tag = %q, want %q", tag, "levelrail/web:abc1234")
	}

	if builder.lastReq.ContextDir != "/repo" {
		t.Errorf("ContextDir = %q, want /repo", builder.lastReq.ContextDir)
	}
	if want := filepath.Join("/repo", "Dockerfile"); builder.lastReq.DockerfilePath != want {
		t.Errorf("DockerfilePath = %q, want %q", builder.lastReq.DockerfilePath, want)
	}
	if builder.lastReq.Tag != "levelrail/web:abc1234" {
		t.Errorf("Tag = %q, want levelrail/web:abc1234", builder.lastReq.Tag)
	}

	if svcStore.saveCalls != 1 {
		t.Fatalf("SaveDesiredService called %d times, want 1", svcStore.saveCalls)
	}
	if svcStore.saved.Name != "web" || svcStore.saved.Image != "levelrail/web:abc1234" || svcStore.saved.Port != 3000 {
		t.Errorf("saved = %+v, want Name=web Image=levelrail/web:abc1234 Port=3000", svcStore.saved)
	}
}

func TestPipeline_Deploy_Dockerfile_BaseDirectory_ScopesContext(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}
	p := New(builder, &fakeServiceStore{})

	svc := dockerfileService()
	svc.Build.BaseDirectory = "apps/web"

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "web",
		Service:     svc,
		SourceDir:   "/repo",
		CommitSHA:   "abc1234",
		ImageRepo:   "levelrail/web",
	}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}

	wantContext := filepath.Join("/repo", "apps/web")
	if builder.lastReq.ContextDir != wantContext {
		t.Errorf("ContextDir = %q, want %q", builder.lastReq.ContextDir, wantContext)
	}
	wantDockerfile := filepath.Join(wantContext, "Dockerfile")
	if builder.lastReq.DockerfilePath != wantDockerfile {
		t.Errorf("DockerfilePath = %q, want %q", builder.lastReq.DockerfilePath, wantDockerfile)
	}
}

func TestPipeline_Deploy_Dockerfile_BaseDirectory_Traversal_Rejected(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}
	p := New(builder, &fakeServiceStore{})

	svc := dockerfileService()
	svc.Build.BaseDirectory = "../escape"

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "web",
		Service:     svc,
		SourceDir:   "/repo",
		CommitSHA:   "abc1234",
		ImageRepo:   "levelrail/web",
	}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want an error for a base directory that escapes the repo root")
	}
	if builder.calls != 0 {
		t.Errorf("Build called %d times, want 0: a traversal must be rejected before any build attempt", builder.calls)
	}
}

func TestPipeline_Deploy_ProgressForwarded(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}
	p := New(builder, &fakeServiceStore{})

	var got []build.ProgressEvent
	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "web", Service: dockerfileService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web",
	}, func(ev build.ProgressEvent) { got = append(got, ev) })
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if len(got) == 0 {
		t.Error("expected at least one progress event forwarded from the builder")
	}
}

func TestPipeline_Deploy_BuildFailure_DoesNotSave(t *testing.T) {
	builder := &fakeBuilder{err: errors.New("build failed")}
	svcStore := &fakeServiceStore{}
	p := New(builder, svcStore)

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "web", Service: dockerfileService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web",
	}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want the build error to propagate")
	}
	if svcStore.saveCalls != 0 {
		t.Errorf("SaveDesiredService called %d times, want 0: a failed build must never reach the store", svcStore.saveCalls)
	}
}

func TestPipeline_Deploy_SaveFailure(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}
	svcStore := &fakeServiceStore{saveErr: errors.New("disk full")}
	p := New(builder, svcStore)

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "web", Service: dockerfileService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web",
	}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want the save error to propagate")
	}
}

func TestPipeline_Deploy_Image_Success(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "should-never-be-used"}}
	svcStore := &fakeServiceStore{}
	p := New(builder, svcStore)

	tag, err := p.Deploy(context.Background(), Request{
		ServiceName: "web",
		Service:     imageService(),
		// SourceDir/CommitSHA/ImageRepo deliberately left unset: a
		// build.type: image deploy needs none of them, there is no
		// checkout and no tag this platform mints.
	}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if tag != "ghcr.io/org/app:v1.2.3" {
		t.Errorf("tag = %q, want %q", tag, "ghcr.io/org/app:v1.2.3")
	}

	if builder.calls != 0 || builder.railpackCalls != 0 {
		t.Errorf("builder called (Build=%d, BuildRailpack=%d), want 0: build.type: image never builds anything", builder.calls, builder.railpackCalls)
	}

	if svcStore.saveCalls != 1 {
		t.Fatalf("SaveDesiredService called %d times, want 1", svcStore.saveCalls)
	}
	if svcStore.saved.Name != "web" || svcStore.saved.Image != "ghcr.io/org/app:v1.2.3" || svcStore.saved.Port != 3000 {
		t.Errorf("saved = %+v, want Name=web Image=ghcr.io/org/app:v1.2.3 Port=3000", svcStore.saved)
	}
}

func TestPipeline_Deploy_Image_SaveFailure(t *testing.T) {
	builder := &fakeBuilder{}
	svcStore := &fakeServiceStore{saveErr: errors.New("disk full")}
	p := New(builder, svcStore)

	_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: imageService()}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want the save error to propagate")
	}
}

func TestPipeline_Deploy_Image_RegistryCredential_Resolved(t *testing.T) {
	builder := &fakeBuilder{}
	svcStore := &fakeServiceStore{
		registryCredentials: map[string]store.RegistryCredential{
			"ghcr-bot": {ID: "regcred_abc123", Name: "ghcr-bot", RegistryHost: "ghcr.io", Username: "bot"},
		},
	}
	p := New(builder, svcStore)

	svc := imageService()
	svc.Build.RegistryCredential = "ghcr-bot"

	_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if svcStore.saved.RegistryCredentialID != "regcred_abc123" {
		t.Errorf("saved.RegistryCredentialID = %q, want %q", svcStore.saved.RegistryCredentialID, "regcred_abc123")
	}
}

func TestPipeline_Deploy_Image_RegistryCredential_NotFound_Rejected(t *testing.T) {
	builder := &fakeBuilder{}
	svcStore := &fakeServiceStore{}
	p := New(builder, svcStore)

	svc := imageService()
	svc.Build.RegistryCredential = "does-not-exist"

	_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want an error for an unknown registry credential name")
	}
	if svcStore.saveCalls != 0 {
		t.Errorf("SaveDesiredService called %d times, want 0: an unresolved credential must never reach the store", svcStore.saveCalls)
	}
}

func TestPipeline_Deploy_Image_UnresolvedEnv_Rejected(t *testing.T) {
	builder := &fakeBuilder{}
	svcStore := &fakeServiceStore{}
	p := New(builder, svcStore)

	svc := imageService()
	svc.Env = map[string]spec.EnvVar{"DATABASE_URL": {From: "postgres.main.url"}}

	_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want the unresolved env error to propagate")
	}
	if svcStore.saveCalls != 0 {
		t.Errorf("SaveDesiredService called %d times, want 0: env validation must run before saving", svcStore.saveCalls)
	}
}

func TestPipeline_Deploy_UnsupportedBuildTypes(t *testing.T) {
	// spec.BuildStatic is deliberately not in this list: it has its own
	// TestPipeline_DeployStatic_* tests in static_test.go, since a
	// successful static deploy needs a StaticSiteStore/static root dir
	// configured, unlike every case here which fails before any of that
	// matters. spec.BuildRailpack is also deliberately not in this list
	// any more: it has its own TestPipeline_Deploy_Railpack_* tests
	// below, now that it's a supported build.type.
	tests := []string{spec.BuildCompose, "not-a-real-type", ""}

	for _, bt := range tests {
		t.Run(bt, func(t *testing.T) {
			builder := &fakeBuilder{result: &build.Result{Tag: "x:y"}}
			p := New(builder, &fakeServiceStore{})

			svc := dockerfileService()
			svc.Build.Type = bt

			_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil)
			if err == nil {
				t.Fatalf("Deploy() error = nil for build.type %q, want an error", bt)
			}
			if builder.calls != 0 {
				t.Errorf("builder.Build called %d times for unsupported build.type %q, want 0", builder.calls, bt)
			}
		})
	}
}

func TestPipeline_Deploy_UnresolvedEnv_RejectedBeforeBuild(t *testing.T) {
	tests := []struct {
		name string
		env  spec.EnvVar
	}{
		{name: "secret", env: spec.EnvVar{Secret: true, Required: true}},
		{name: "from reference", env: spec.EnvVar{From: "postgres.main.url"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &fakeBuilder{result: &build.Result{Tag: "x:y"}}
			svcStore := &fakeServiceStore{}
			p := New(builder, svcStore)

			svc := dockerfileService()
			svc.Env = map[string]spec.EnvVar{"SECRET_VAR": tt.env}

			_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil)
			if err == nil {
				t.Fatal("Deploy() error = nil, want unresolved env to be rejected")
			}
			if builder.calls != 0 {
				t.Errorf("builder.Build called %d times, want 0: must fail before attempting a build", builder.calls)
			}
			if svcStore.saveCalls != 0 {
				t.Errorf("SaveDesiredService called %d times, want 0", svcStore.saveCalls)
			}
		})
	}
}

func TestPipeline_Deploy_FromReference_UnknownDatabase_Rejected(t *testing.T) {
	// A { from: ... } reference to a database that doesn't exist is
	// rejected at deploy time regardless of whether a SecretChecker is
	// configured: the database itself, not the secret store, is missing.
	builder := &fakeBuilder{result: &build.Result{Tag: "x:y"}}
	svcStore := &fakeServiceStore{}
	checker := newFakeSecretChecker()
	p := New(builder, svcStore, WithSecretChecker(checker))

	svc := dockerfileService()
	svc.Env = map[string]spec.EnvVar{"DATABASE_URL": {From: "postgres.main.url"}}

	_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want a reference to an unknown database to be rejected")
	}
	if builder.calls != 0 {
		t.Errorf("builder.Build called %d times, want 0", builder.calls)
	}
}

func TestPipeline_Deploy_FromReference_UnsupportedField_Rejected(t *testing.T) {
	// redis has no username/password (internal/reconcile/database's own
	// package doc comment): a { from: "main.username" } reference against
	// a redis database is rejected even though the database itself
	// exists.
	builder := &fakeBuilder{result: &build.Result{Tag: "x:y"}}
	svcStore := &fakeServiceStore{databases: map[string]store.DesiredDatabase{
		"main": {Name: "main", Engine: store.EngineRedis},
	}}
	p := New(builder, svcStore)

	svc := dockerfileService()
	svc.Env = map[string]spec.EnvVar{"DB_USER": {From: "main.username"}}

	_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want an unsupported field to be rejected")
	}
	if builder.calls != 0 {
		t.Errorf("builder.Build called %d times, want 0", builder.calls)
	}
}

func TestPipeline_Deploy_FromReference_KnownDatabase_Resolves(t *testing.T) {
	// A { from: ... } reference to a real, existing database now
	// succeeds at deploy time: the declaration (which env var maps to
	// which database/field) is saved on the desired service, resolved to
	// a real value later by internal/reconcile/application immediately
	// before container creation.
	builder := &fakeBuilder{result: &build.Result{Tag: "x:y"}}
	svcStore := &fakeServiceStore{databases: map[string]store.DesiredDatabase{
		"main": {Name: "main", Engine: store.EnginePostgres},
	}}
	p := New(builder, svcStore)

	svc := dockerfileService()
	svc.Env = map[string]spec.EnvVar{"DATABASE_URL": {From: "postgres.main.url"}}

	if _, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil); err != nil {
		t.Fatalf("Deploy() error = %v, want a known database reference to resolve", err)
	}
	if builder.calls != 1 {
		t.Errorf("builder.Build called %d times, want 1", builder.calls)
	}
	got, ok := svcStore.saved.DatabaseEnv["DATABASE_URL"]
	if !ok {
		t.Fatalf("saved.DatabaseEnv[%q] missing, want an entry", "DATABASE_URL")
	}
	want := store.DatabaseEnvRef{Database: "main", Field: "url"}
	if got != want {
		t.Errorf("saved.DatabaseEnv[%q] = %+v, want %+v", "DATABASE_URL", got, want)
	}
	if _, literal := svcStore.saved.Env["DATABASE_URL"]; literal {
		t.Errorf("saved.Env contains %q, want it absent: a { from: ... } var is never a literal value", "DATABASE_URL")
	}
}

func TestPipeline_Deploy_RequiredSecret_NoValueSet_Rejected(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "x:y"}}
	svcStore := &fakeServiceStore{}
	checker := newFakeSecretChecker() // nothing set
	p := New(builder, svcStore, WithSecretChecker(checker))

	svc := dockerfileService()
	svc.Env = map[string]spec.EnvVar{"API_KEY": {Secret: true, Required: true}}

	_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want a required-but-unset secret to be rejected")
	}
	if builder.calls != 0 {
		t.Errorf("builder.Build called %d times, want 0: must fail before attempting a build", builder.calls)
	}
	if checker.lastService != "web" || checker.lastEnvKey != "API_KEY" {
		t.Errorf("checker called with (%q, %q), want (web, API_KEY)", checker.lastService, checker.lastEnvKey)
	}
}

func TestPipeline_Deploy_RequiredSecret_ValueSet_PassesThrough(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "x:y"}}
	svcStore := &fakeServiceStore{}
	checker := newFakeSecretChecker("web/API_KEY")
	p := New(builder, svcStore, WithSecretChecker(checker))

	svc := dockerfileService()
	svc.Env = map[string]spec.EnvVar{"API_KEY": {Secret: true, Required: true}}

	_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v, want it to pass now that a value is set", err)
	}
	if builder.calls != 1 {
		t.Errorf("builder.Build called %d times, want 1", builder.calls)
	}
	if _, exists := svcStore.saved.Env["API_KEY"]; exists {
		t.Error("saved.Env contains API_KEY, want secret-backed vars kept out of the plain Env map entirely")
	}
	if len(svcStore.saved.SecretEnv) != 1 || svcStore.saved.SecretEnv[0] != "API_KEY" {
		t.Errorf("saved.SecretEnv = %v, want [API_KEY]", svcStore.saved.SecretEnv)
	}
}

func TestPipeline_Deploy_OptionalSecret_NoValueSet_StillPassesThrough(t *testing.T) {
	// Required=false means "ok to be unset", matching spec.EnvVar's own
	// documented meaning: only a required-but-unset secret fails the
	// deploy.
	builder := &fakeBuilder{result: &build.Result{Tag: "x:y"}}
	svcStore := &fakeServiceStore{}
	checker := newFakeSecretChecker() // nothing set
	p := New(builder, svcStore, WithSecretChecker(checker))

	svc := dockerfileService()
	svc.Env = map[string]spec.EnvVar{"OPTIONAL_FLAG": {Secret: true, Required: false}}

	_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v, want an optional unset secret to pass through", err)
	}
	if len(svcStore.saved.SecretEnv) != 1 || svcStore.saved.SecretEnv[0] != "OPTIONAL_FLAG" {
		t.Errorf("saved.SecretEnv = %v, want [OPTIONAL_FLAG]", svcStore.saved.SecretEnv)
	}
}

func TestPipeline_Deploy_SecretChecker_ErrorPropagates(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "x:y"}}
	checker := newFakeSecretChecker()
	checker.err = errors.New("database unavailable")
	p := New(builder, &fakeServiceStore{}, WithSecretChecker(checker))

	svc := dockerfileService()
	svc.Env = map[string]spec.EnvVar{"API_KEY": {Secret: true}}

	_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want the checker's error to propagate")
	}
	if builder.calls != 0 {
		t.Errorf("builder.Build called %d times, want 0", builder.calls)
	}
}

func TestPipeline_Deploy_LiteralEnv_PassesThrough(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "x:y"}}
	svcStore := &fakeServiceStore{}
	p := New(builder, svcStore)

	svc := dockerfileService()
	svc.Env = map[string]spec.EnvVar{"NODE_ENV": {Value: "production"}}

	_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if svcStore.saved.Env["NODE_ENV"] != "production" {
		t.Errorf("saved.Env = %+v, want NODE_ENV=production", svcStore.saved.Env)
	}
}

func TestPipeline_Deploy_NoDockerfilePathDefaultsEmpty(t *testing.T) {
	// build.Path unset in app.yaml: internal/build defaults to
	// "<ContextDir>/Dockerfile" itself, so this package must pass an
	// empty DockerfilePath through, not invent one.
	builder := &fakeBuilder{result: &build.Result{Tag: "x:y"}}
	p := New(builder, &fakeServiceStore{})

	svc := spec.Service{Build: spec.Build{Type: spec.BuildDockerfile}, Port: 8080}
	_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if builder.lastReq.DockerfilePath != "" {
		t.Errorf("DockerfilePath = %q, want empty (internal/build applies its own default)", builder.lastReq.DockerfilePath)
	}
}

func TestPipeline_Deploy_RecordsBuildDuration(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234", Duration: 42 * time.Second}}
	svcStore := &fakeServiceStore{}
	recorder := &fakeBuildMetricsRecorder{}
	p := New(builder, svcStore, WithBuildMetricsRecorder(recorder))

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "web", Service: dockerfileService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web",
	}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if recorder.calls != 1 {
		t.Fatalf("RecordBuildDuration called %d times, want 1", recorder.calls)
	}
	if recorder.lastServiceName != "web" {
		t.Errorf("recorded service = %q, want web", recorder.lastServiceName)
	}
	if recorder.lastDuration != 42*time.Second {
		t.Errorf("recorded duration = %v, want 42s", recorder.lastDuration)
	}
}

func TestPipeline_Deploy_NoRecorderConfigured_StillSucceeds(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234", Duration: time.Second}}
	p := New(builder, &fakeServiceStore{})

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "web", Service: dockerfileService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web",
	}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v, want nil: no recorder configured is a valid default", err)
	}
}

func TestPipeline_Deploy_RecorderError_DoesNotFailDeploy(t *testing.T) {
	// A metrics-store hiccup must never undo a deploy that already
	// succeeded (build done, desired state already saved): the recorder
	// call happens last and its error is swallowed (logged), not
	// returned, matching reconcile.Engine.reconcileOne's own choice for
	// persisting reconcile status.
	builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234", Duration: time.Second}}
	svcStore := &fakeServiceStore{}
	recorder := &fakeBuildMetricsRecorder{err: errors.New("telemetry store unavailable")}
	var logBuf strings.Builder
	p := New(builder, svcStore, WithBuildMetricsRecorder(recorder), WithLogger(slog.New(slog.NewTextHandler(&logBuf, nil))))

	tag, err := p.Deploy(context.Background(), Request{
		ServiceName: "web", Service: dockerfileService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web",
	}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v, want nil: a recorder failure must not fail an already-successful deploy", err)
	}
	if tag != "levelrail/web:abc1234" {
		t.Errorf("tag = %q, want levelrail/web:abc1234", tag)
	}
	if svcStore.saveCalls != 1 {
		t.Errorf("SaveDesiredService called %d times, want 1", svcStore.saveCalls)
	}
	if !strings.Contains(logBuf.String(), "record build duration metric failed") {
		t.Errorf("log output = %q, want it to mention the recorder failure", logBuf.String())
	}
}

// --- spec.BuildRailpack: deployRailpack, mirroring the
// TestPipeline_Deploy_Dockerfile_* / _ProgressForwarded / _*Failure
// tests above for the Dockerfile path, substituting
// fakeBuilder.BuildRailpack for fakeBuilder.Build throughout.

func TestPipeline_Deploy_Railpack_Success(t *testing.T) {
	builder := &fakeBuilder{railpackResult: &build.Result{Tag: "levelrail/web:abc1234"}}
	svcStore := &fakeServiceStore{}
	p := New(builder, svcStore)

	tag, err := p.Deploy(context.Background(), Request{
		ServiceName: "web",
		Service:     railpackService(),
		SourceDir:   "/repo",
		CommitSHA:   "abc1234",
		ImageRepo:   "levelrail/web",
	}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if tag != "levelrail/web:abc1234" {
		t.Errorf("tag = %q, want %q", tag, "levelrail/web:abc1234")
	}

	if builder.railpackCalls != 1 {
		t.Fatalf("BuildRailpack called %d times, want 1", builder.railpackCalls)
	}
	if builder.calls != 0 {
		t.Errorf("Build (Dockerfile path) called %d times, want 0", builder.calls)
	}
	if builder.lastRailpackReq.SourceDir != "/repo" {
		t.Errorf("SourceDir = %q, want /repo", builder.lastRailpackReq.SourceDir)
	}
	if builder.lastRailpackReq.Tag != "levelrail/web:abc1234" {
		t.Errorf("Tag = %q, want levelrail/web:abc1234", builder.lastRailpackReq.Tag)
	}

	if svcStore.saveCalls != 1 {
		t.Fatalf("SaveDesiredService called %d times, want 1", svcStore.saveCalls)
	}
	if svcStore.saved.Name != "web" || svcStore.saved.Image != "levelrail/web:abc1234" || svcStore.saved.Port != 3000 {
		t.Errorf("saved = %+v, want Name=web Image=levelrail/web:abc1234 Port=3000", svcStore.saved)
	}
}

func TestPipeline_Deploy_Railpack_BaseDirectory_ScopesSourceDir(t *testing.T) {
	builder := &fakeBuilder{railpackResult: &build.Result{Tag: "levelrail/web:abc1234"}}
	p := New(builder, &fakeServiceStore{})

	svc := railpackService()
	svc.Build.BaseDirectory = "apps/web"

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "web",
		Service:     svc,
		SourceDir:   "/repo",
		CommitSHA:   "abc1234",
		ImageRepo:   "levelrail/web",
	}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}

	want := filepath.Join("/repo", "apps/web")
	if builder.lastRailpackReq.SourceDir != want {
		t.Errorf("SourceDir = %q, want %q", builder.lastRailpackReq.SourceDir, want)
	}
}

func TestPipeline_Deploy_Railpack_ProgressForwarded(t *testing.T) {
	builder := &fakeBuilder{railpackResult: &build.Result{Tag: "levelrail/web:abc1234"}}
	p := New(builder, &fakeServiceStore{})

	var got []build.ProgressEvent
	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "web", Service: railpackService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web",
	}, func(ev build.ProgressEvent) { got = append(got, ev) })
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if len(got) == 0 {
		t.Error("expected at least one progress event forwarded from the builder")
	}
}

func TestPipeline_Deploy_Railpack_BuildFailure_DoesNotSave(t *testing.T) {
	builder := &fakeBuilder{railpackErr: errors.New("build failed")}
	svcStore := &fakeServiceStore{}
	p := New(builder, svcStore)

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "web", Service: railpackService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web",
	}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want the build error to propagate")
	}
	if svcStore.saveCalls != 0 {
		t.Errorf("SaveDesiredService called %d times, want 0: a failed build must never reach the store", svcStore.saveCalls)
	}
}

func TestPipeline_Deploy_Railpack_SaveFailure(t *testing.T) {
	builder := &fakeBuilder{railpackResult: &build.Result{Tag: "levelrail/web:abc1234"}}
	svcStore := &fakeServiceStore{saveErr: errors.New("disk full")}
	p := New(builder, svcStore)

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "web", Service: railpackService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web",
	}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want the save error to propagate")
	}
}

// TestPipeline_Deploy_Railpack_UnsupportedProvider_ExplicitMessage is this
// slice's load-bearing "fail loudly, not silently" assertion
// (docs-local/research/railpack-integration-decision.md's recommendation
// section): when internal/build reports Railpack detected a provider
// outside supportedRailpackProviders, deployRailpack must surface the
// service name and the detected provider explicitly, not a generic build
// failure, matching validateEnv's own tone for unsupported env
// resolution.
func TestPipeline_Deploy_Railpack_UnsupportedProvider_ExplicitMessage(t *testing.T) {
	builder := &fakeBuilder{railpackErr: &build.UnsupportedProviderError{Provider: "python"}}
	svcStore := &fakeServiceStore{}
	p := New(builder, svcStore)

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "web", Service: railpackService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web",
	}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want the unsupported provider to be rejected")
	}
	want := `deploy: service "web": railpack detected provider "python", only node and golang are supported yet`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if svcStore.saveCalls != 0 {
		t.Errorf("SaveDesiredService called %d times, want 0", svcStore.saveCalls)
	}
}

// TestPipeline_Deploy_Railpack_NoProviderDetected_ExplicitMessage is the
// same assertion as above for the "Railpack detected nothing at all"
// case (build.UnsupportedProviderError.Provider == "").
func TestPipeline_Deploy_Railpack_NoProviderDetected_ExplicitMessage(t *testing.T) {
	builder := &fakeBuilder{railpackErr: &build.UnsupportedProviderError{}}
	p := New(builder, &fakeServiceStore{})

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "web", Service: railpackService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web",
	}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want the unsupported provider to be rejected")
	}
	want := `deploy: service "web": railpack detected provider "(none detected)", only node and golang are supported yet`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestPipeline_Deploy_Railpack_UnresolvedEnv_RejectedBeforeBuild(t *testing.T) {
	builder := &fakeBuilder{railpackResult: &build.Result{Tag: "x:y"}}
	svcStore := &fakeServiceStore{}
	p := New(builder, svcStore)

	svc := railpackService()
	svc.Env = map[string]spec.EnvVar{"SECRET_VAR": {Secret: true, Required: true}}

	_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want unresolved env to be rejected")
	}
	if builder.railpackCalls != 0 {
		t.Errorf("BuildRailpack called %d times, want 0: must fail before attempting a build", builder.railpackCalls)
	}
	if svcStore.saveCalls != 0 {
		t.Errorf("SaveDesiredService called %d times, want 0", svcStore.saveCalls)
	}
}

func TestPipeline_Deploy_Railpack_RecordsBuildDuration(t *testing.T) {
	builder := &fakeBuilder{railpackResult: &build.Result{Tag: "levelrail/web:abc1234", Duration: 42 * time.Second}}
	svcStore := &fakeServiceStore{}
	recorder := &fakeBuildMetricsRecorder{}
	p := New(builder, svcStore, WithBuildMetricsRecorder(recorder))

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "web", Service: railpackService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web",
	}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if recorder.calls != 1 {
		t.Fatalf("RecordBuildDuration called %d times, want 1", recorder.calls)
	}
	if recorder.lastDuration != 42*time.Second {
		t.Errorf("recorded duration = %v, want 42s", recorder.lastDuration)
	}
}
