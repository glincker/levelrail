package deploy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// cancelingBuilder reports partial progress, then aborts the way BuildKit
// does when its context is cancelled mid-solve.
type cancelingBuilder struct{ fakeBuilder }

func (c *cancelingBuilder) Build(ctx context.Context, _ build.Request, progress func(build.ProgressEvent)) (*build.Result, error) {
	if progress != nil {
		progress(build.ProgressEvent{Log: "step 1/4 done", Stream: "stdout"})
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestPipeline_Deploy_BuildFailureModes_NeverSaveDesiredState(t *testing.T) {
	tests := []struct {
		name      string
		buildType string
		dockerErr error
		wantSub   string
	}{
		{"dockerfile: daemon unreachable", spec.BuildDockerfile, errors.New("Cannot connect to the Docker daemon"), "Cannot connect to the Docker daemon"},
		{"dockerfile: image load fails after build", spec.BuildDockerfile, errors.New("load image: broken pipe"), "load image: broken pipe"},
		{"dockerfile: out of disk", spec.BuildDockerfile, errors.New("no space left on device"), "no space left on device"},
		{"railpack: daemon unreachable", spec.BuildRailpack, errors.New("Cannot connect to the Docker daemon"), "Cannot connect to the Docker daemon"},
		{"railpack: corrupt layer", spec.BuildRailpack, errors.New("layer sha256:abc: digest mismatch"), "digest mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &fakeBuilder{err: tt.dockerErr, railpackErr: tt.dockerErr}
			svcStore := &fakeServiceStore{}
			p := New(builder, svcStore)

			svc := dockerfileService()
			if tt.buildType == spec.BuildRailpack {
				svc = railpackService()
			}
			var events int
			_, err := p.Deploy(context.Background(), Request{
				ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web",
			}, func(build.ProgressEvent) { events++ })
			if err == nil {
				t.Fatal("Deploy() error = nil, want the build failure")
			}
			if !strings.Contains(err.Error(), tt.wantSub) || !strings.Contains(err.Error(), `service "web"`) {
				t.Errorf("Deploy() error = %q, want service name and %q", err, tt.wantSub)
			}
			if !errors.Is(err, tt.dockerErr) {
				t.Errorf("Deploy() error does not wrap the builder error: %v", err)
			}
			if events == 0 {
				t.Error("progress callback never invoked: partial build output must reach the caller before the failure")
			}
			if svcStore.saveCalls != 0 {
				t.Errorf("SaveDesiredService called %d times, want 0", svcStore.saveCalls)
			}
		})
	}
}

func TestPipeline_Deploy_ContextCancelledMidBuild_NoSaveAndErrorIsCancellation(t *testing.T) {
	builder := &cancelingBuilder{}
	svcStore := &fakeServiceStore{}
	p := New(builder, svcStore)

	ctx, cancel := context.WithCancel(context.Background())
	var logged []string
	done := make(chan error, 1)
	go func() {
		_, err := p.Deploy(ctx, Request{
			ServiceName: "web", Service: dockerfileService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web",
		}, func(ev build.ProgressEvent) {
			if ev.Log != "" {
				logged = append(logged, ev.Log)
			}
		})
		done <- err
	}()
	cancel()

	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Deploy() error = %v, want it to wrap context.Canceled so callers can tell shutdown from a real failure", err)
	}
	if len(logged) != 1 {
		t.Errorf("logged = %v, want the partial output preserved", logged)
	}
	if svcStore.saveCalls != 0 {
		t.Errorf("SaveDesiredService called %d times, want 0 after cancellation", svcStore.saveCalls)
	}
}

func TestPipeline_Deploy_SaveFailsAfterBuild_ReportsAndDoesNotRecordMetric(t *testing.T) {
	recorder := &fakeBuildMetricsRecorder{}
	builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}
	svcStore := &fakeServiceStore{saveErr: errors.New("database is locked")}
	p := New(builder, svcStore, WithBuildMetricsRecorder(recorder))

	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "web", Service: dockerfileService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "save desired state") || !errors.Is(err, svcStore.saveErr) {
		t.Fatalf("Deploy() error = %v, want a wrapped save failure", err)
	}
	if recorder.calls != 0 {
		t.Errorf("build metric recorded %d times, want 0: the deploy did not complete", recorder.calls)
	}
}
