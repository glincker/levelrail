package deploy

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestPipeline_Deploy_Live_ThenReconciles is the real end-to-end proof
// for TASKS.md 1.4, and for how it closes the loop with 1.3: real
// BuildKit builds a real image from this package's testdata Dockerfile,
// the pipeline saves it as desired state in a real SQLite store, and the
// application controller (internal/reconcile/application, built under
// 1.3) actually converges a real running container from it, exactly the
// chain a git push will drive once 1.5 (git integration) exists.
func TestPipeline_Deploy_Live_ThenReconciles(t *testing.T) {
	dockertest.SkipIfShort(t)
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

	runtime, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("closing docker.Client: %v", err)
		}
	})

	const serviceName = "levelrail-test-deploy-pipeline"
	repo := "levelrail/test-deploy-pipeline"
	sha := "livetest1"
	tag := repo + ":" + sha

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = dockerCli.ImageRemove(cleanupCtx, tag, image.RemoveOptions{Force: true})
	})

	svcStore := openLiveStore(t)

	svc := spec.Service{
		Build: spec.Build{Type: spec.BuildDockerfile},
		Port:  0, // this fixture has nothing listening; port 0 means the application controller skips probing entirely
	}

	pipeline := New(buildClient, svcStore)

	deployCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	builtTag, err := pipeline.Deploy(deployCtx, Request{
		ServiceName: serviceName,
		Service:     svc,
		SourceDir:   "testdata",
		CommitSHA:   sha,
		ImageRepo:   repo,
	}, nil)
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if builtTag != tag {
		t.Fatalf("Deploy() returned tag %q, want %q", builtTag, tag)
	}

	saved, err := svcStore.GetDesiredService(deployCtx, serviceName)
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if saved.Image != tag {
		t.Fatalf("saved desired state Image = %q, want %q", saved.Image, tag)
	}

	// This is the actual closure of the loop: the application controller
	// built under 1.3, pointed at the same store and runtime, reconciling
	// the desired state Deploy() just wrote, with no glue code in
	// between beyond what both packages already independently expose.
	runtimeCleanupCtx := context.Background()
	found, _ := runtime.ListByPrefix(runtimeCleanupCtx, serviceName+"-")
	for _, cs := range found {
		_ = runtime.Stop(runtimeCleanupCtx, cs.ID, 3*time.Second)
		_ = runtime.Remove(runtimeCleanupCtx, cs.ID, true)
	}
	t.Cleanup(func() {
		found, _ := runtime.ListByPrefix(context.Background(), serviceName+"-")
		for _, cs := range found {
			_ = runtime.Stop(context.Background(), cs.ID, 3*time.Second)
			_ = runtime.Remove(context.Background(), cs.ID, true)
		}
	})

	ctrl := application.New(serviceName, svcStore, runtime)
	result, err := ctrl.Reconcile(deployCtx)
	if err != nil {
		t.Fatalf("application Controller.Reconcile() error = %v, result = %+v", err, result)
	}
	if len(result.Conditions) == 0 || result.Conditions[0].Status != "True" {
		t.Fatalf("Reconcile() result = %+v, want a True Ready condition", result)
	}

	// Independent verification, not trusting the returned Result: the
	// container this whole chain was supposed to produce actually exists
	// and is running, built from exactly the image Deploy() built.
	containers, err := runtime.ListByPrefix(deployCtx, serviceName+"-")
	if err != nil {
		t.Fatalf("ListByPrefix() error = %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected exactly 1 container for %q after reconcile, got %d: %+v", serviceName, len(containers), containers)
	}
	if !containers[0].Running {
		t.Errorf("container %+v is not running", containers[0])
	}
	if containers[0].Image != tag {
		t.Errorf("running container's image = %q, want the image Deploy() built: %q", containers[0].Image, tag)
	}
}

func openLiveStore(t *testing.T) *store.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "levelrail.db")
	db, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing store: %v", err)
		}
	})
	return db
}
