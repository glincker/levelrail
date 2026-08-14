package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestRollback_Live_ImageSwapBothDirections proves the claim
// deploy_test.go's own package doc comment names as an explicit gap
// ("no rollback"), and TASKS.md 1.3 and internal/api/deploys.go's
// handleTriggerDeploy doc comment both describe: rollback is not a
// separate code path, it is the exact same "point desired.Image at a
// tag and reconcile" mechanism run with an older tag as input. There is
// nothing rollback-specific in internal/reconcile/application to test in
// isolation; what has to be proven live is that the forward mechanism
// genuinely reverses, not just that a second API call returns success.
//
// Two builds of the identical fixture content under two distinct tags,
// not two different fixture directories. application.ContainerName
// derives a container's name from a sha256 of its image string (see
// internal/reconcile/application/controller.go), so two different tags
// already produce two genuinely different, independently-tracked
// containers regardless of what's inside the image. A second fixture
// with a different response body would only prove that HTTP traffic
// reaches a different container, a fact already established by
// TestDeploy_Live_BuildToHTTPS; it would add an ingress/Caddy/HTTPS
// layer to this test without adding any coverage of the thing rollback
// actually is, which is container identity swapping under
// application.Controller.Reconcile. Proof here is direct: InspectByName
// and ListByPrefix against the real container image tag Docker reports,
// stronger than an HTTP response body would be and without the
// dependency on a second, differently-authored fixture.
//
// Skips cleanly, does not fail, if Docker or BuildKit aren't reachable,
// matching TestDeploy_Live_BuildToHTTPS's exact pattern.
func TestRollback_Live_ImageSwapBothDirections(t *testing.T) {
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

	// A prefix distinct from every other e2e test's service name
	// (TestDeploy_Live_BuildToHTTPS uses "levelrail-test-e2e-deploy"),
	// so sibling e2e suites can run concurrently against the same local
	// Docker daemon without colliding on container or image names.
	const serviceName = "levelrail-test-e2e-rollback"
	repo := "levelrail/test-e2e-rollback"
	tagA := repo + ":e2elivetestrollbacka"
	tagB := repo + ":e2elivetestrollbackb"

	// Image cleanup: registered before either build runs, matching
	// deploy_test.go's "clean before, clean after" convention, so a
	// failure partway through this test still leaves nothing behind on
	// a re-run.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = dockerCli.ImageRemove(cleanupCtx, tagA, image.RemoveOptions{Force: true})
		_, _ = dockerCli.ImageRemove(cleanupCtx, tagB, image.RemoveOptions{Force: true})
	})

	cleanupContainers(context.Background(), t, runtime, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, runtime, serviceName) })

	svcStore := openLiveStore(t)

	// One shared budget for two builds and three reconciles: a bigger
	// window than deploy_test.go's single-build 3 minutes, since this
	// test does strictly more work against the same daemon.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Step 1: build image A, the same fixture deploy_test.go uses.
	resA, err := buildClient.Build(ctx, build.Request{
		ContextDir: "../fixtures/hello-e2e",
		Tag:        tagA,
	}, nil)
	if err != nil {
		t.Fatalf("Build(A) error = %v", err)
	}
	if resA.Tag != tagA {
		t.Fatalf("Build(A) returned tag %q, want %q", resA.Tag, tagA)
	}

	// Step 2: build image B, the identical fixture content under a
	// distinct tag. What is being tested is container identity keyed by
	// image (application.ContainerName), not the response body: see the
	// package-level comment above for why a second fixture directory
	// would not add coverage here.
	resB, err := buildClient.Build(ctx, build.Request{
		ContextDir: "../fixtures/hello-e2e",
		Tag:        tagB,
	}, nil)
	if err != nil {
		t.Fatalf("Build(B) error = %v", err)
	}
	if resB.Tag != tagB {
		t.Fatalf("Build(B) returned tag %q, want %q", resB.Tag, tagB)
	}

	appCtrl := application.New(serviceName, svcStore, runtime)
	nameA := application.ContainerName(serviceName, tagA)
	nameB := application.ContainerName(serviceName, tagB)

	// Step 3: deploy A. Save desired state pointed at tagA and
	// reconcile through a real application.Controller, then verify
	// directly against Docker, not just the returned Result, that the
	// container matching A's tag is running.
	deployAndVerify(ctx, t, svcStore, appCtrl, runtime, serviceName, tagA, nameA)

	// Step 4: deploy B, a real second, distinct build, over the top of
	// A. This proves the forward path: A's container is genuinely gone,
	// not merely superseded, by the time this returns.
	deployAndVerify(ctx, t, svcStore, appCtrl, runtime, serviceName, tagB, nameB)
	assertContainerAbsent(ctx, t, runtime, nameA, "A")

	// Step 5: "roll back". Point desired.Image back at A's exact tag,
	// already built in step 1, no rebuild needed, and reconcile a third
	// time. This is the actual claim under test: that pointing
	// desired.Image back at an older tag and reconciling converges to
	// it the same way any other redeploy does. Steps 3-4 only establish
	// that the forward path works at all, so a regression here is
	// unambiguously a rollback-specific failure, not a general
	// reconcile failure.
	deployAndVerify(ctx, t, svcStore, appCtrl, runtime, serviceName, tagA, nameA)
	assertContainerAbsent(ctx, t, runtime, nameB, "B")
}

// deployAndVerify points serviceName's desired image at tag, reconciles
// through appCtrl, and verifies directly against runtime (not the
// returned Result) that exactly one container exists for the service,
// it is running, its image is tag, and InspectByName for the
// image-derived container name agrees. Shared by all three deploys in
// TestRollback_Live_ImageSwapBothDirections (A, then B, then back to A)
// so the same rigor is applied to the rollback step as to the two
// forward deploys that set it up.
func deployAndVerify(
	ctx context.Context,
	t *testing.T,
	svcStore *store.DB,
	appCtrl *application.Controller,
	runtime docker.Runtime,
	serviceName, tag, wantContainerName string,
) {
	t.Helper()

	desired := store.DesiredService{
		Name:  serviceName,
		Image: tag,
		Port:  8080,
		Health: &store.ServiceHealth{
			Readiness: &store.ServiceProbe{Path: "/"},
		},
	}
	if err := svcStore.SaveDesiredService(ctx, desired); err != nil {
		t.Fatalf("SaveDesiredService(%q) error = %v", tag, err)
	}

	result, err := appCtrl.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Controller.Reconcile() for %q error = %v, result = %+v", tag, err, result)
	}
	if len(result.Conditions) == 0 || result.Conditions[0].Status != "True" {
		t.Fatalf("Controller.Reconcile() for %q result = %+v, want a True Ready condition", tag, result)
	}

	containers, err := runtime.ListByPrefix(ctx, serviceName+"-")
	if err != nil {
		t.Fatalf("ListByPrefix() error = %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected exactly 1 container for %q after reconciling to %q, got %d: %+v", serviceName, tag, len(containers), containers)
	}
	if !containers[0].Running {
		t.Fatalf("container %+v is not running", containers[0])
	}
	if containers[0].Image != tag {
		t.Fatalf("running container's image = %q, want %q", containers[0].Image, tag)
	}
	if containers[0].Name != wantContainerName {
		t.Fatalf("running container's name = %q, want %q (application.ContainerName(%q, %q))", containers[0].Name, wantContainerName, serviceName, tag)
	}

	state, err := runtime.InspectByName(ctx, wantContainerName)
	if err != nil {
		t.Fatalf("InspectByName(%q) error = %v", wantContainerName, err)
	}
	if state == nil || !state.Running {
		t.Fatalf("InspectByName(%q) = %+v, want a running container", wantContainerName, state)
	}
}

// assertContainerAbsent verifies removeStale actually removed the
// superseded container, not merely that a different one is now running
// alongside it. label is only used in the failure message.
func assertContainerAbsent(ctx context.Context, t *testing.T, runtime docker.Runtime, containerName, label string) {
	t.Helper()

	state, err := runtime.InspectByName(ctx, containerName)
	if err != nil {
		t.Fatalf("InspectByName(%q) error = %v", containerName, err)
	}
	if state != nil {
		t.Fatalf("container %s (%s) still exists after being superseded, want it removed by removeStale: %+v", label, containerName, state)
	}
}
