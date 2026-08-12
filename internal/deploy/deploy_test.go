package deploy

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

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
