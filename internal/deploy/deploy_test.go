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

type fakeServiceStore struct {
	saveErr   error
	saved     store.DesiredService
	saveCalls int
}

func (f *fakeServiceStore) SaveDesiredService(_ context.Context, svc store.DesiredService) error {
	f.saveCalls++
	f.saved = svc
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

func TestPipeline_Deploy_UnsupportedBuildTypes(t *testing.T) {
	tests := []string{spec.BuildStatic, spec.BuildCompose, spec.BuildRailpack, "not-a-real-type", ""}

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

func TestPipeline_Deploy_FromReference_StillRejected_EvenWithSecretChecker(t *testing.T) {
	// { from: ... } stays unsupported regardless of whether a
	// SecretChecker is configured: it needs managed database
	// credentials, a different, still-blocked dependency.
	builder := &fakeBuilder{result: &build.Result{Tag: "x:y"}}
	svcStore := &fakeServiceStore{}
	checker := newFakeSecretChecker()
	p := New(builder, svcStore, WithSecretChecker(checker))

	svc := dockerfileService()
	svc.Env = map[string]spec.EnvVar{"DATABASE_URL": {From: "postgres.main.url"}}

	_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil)
	if err == nil {
		t.Fatal("Deploy() error = nil, want { from: ... } to still be rejected")
	}
	if builder.calls != 0 {
		t.Errorf("builder.Build called %d times, want 0", builder.calls)
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
