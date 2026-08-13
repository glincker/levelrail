package deploy

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// TestPipeline_Deploy_Live_RecordsBuildDuration is TASKS.md 2.1's
// remaining-gap proof for build duration: a real BuildKit build against
// this package's testdata Dockerfile, wired to a real telemetry.DB via
// WithBuildMetricsRecorder, actually produces a queryable
// build_duration_seconds sample with the build's real wall-clock
// duration, not a fabricated or zero value.
func TestPipeline_Deploy_Live_RecordsBuildDuration(t *testing.T) {
	dockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	t.Cleanup(func() { _ = dockerCli.Close() })

	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, err = dockerCli.Ping(pingCtx)
	cancel()
	if err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	connectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	buildClient, err := build.NewClient(connectCtx, dockerCli)
	cancel()
	if err != nil {
		t.Skipf("could not connect to buildkit: %v", err)
	}
	t.Cleanup(func() { _ = buildClient.Close() })

	const serviceName = "levelrail-test-build-duration-metric"
	repo := "levelrail/test-build-duration-metric"
	sha := "livetest1"
	tag := repo + ":" + sha

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = dockerCli.ImageRemove(cleanupCtx, tag, image.RemoveOptions{Force: true})
	})

	svcStore := openLiveStore(t)

	telemetryDB, err := telemetry.Open(context.Background(), filepath.Join(t.TempDir(), "telemetry.db"))
	if err != nil {
		t.Fatalf("telemetry.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := telemetryDB.Close(); err != nil {
			t.Errorf("closing telemetry store: %v", err)
		}
	})

	pipeline := New(buildClient, svcStore, WithBuildMetricsRecorder(telemetryDB))

	svc := spec.Service{
		Build: spec.Build{Type: spec.BuildDockerfile},
		Port:  0,
	}

	before := time.Now()
	deployCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if _, err := pipeline.Deploy(deployCtx, Request{
		ServiceName: serviceName,
		Service:     svc,
		SourceDir:   "testdata",
		CommitSHA:   sha,
		ImageRepo:   repo,
	}, nil); err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	after := time.Now()

	samples, err := telemetryDB.Query(deployCtx, "service:"+serviceName, telemetry.MetricBuildDuration, before.Add(-time.Second), after.Add(time.Second))
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("Query() = %d samples, want 1", len(samples))
	}
	if samples[0].Value <= 0 {
		t.Errorf("samples[0].Value = %v, want a positive build duration in seconds", samples[0].Value)
	}
}
