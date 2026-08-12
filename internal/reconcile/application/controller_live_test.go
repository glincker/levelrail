package application

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_Reconcile_Live is the real end-to-end proof for this
// controller: a real store, a real Docker daemon, a real container, a
// real HTTP readiness probe against it, and a real blue-green cutover
// when the desired image changes, verified by actually inspecting what's
// running before and after, not by trusting the returned Result alone.
func TestController_Reconcile_Live(t *testing.T) {
	rt, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	rawCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	if _, err := rawCli.Ping(ctx); err != nil {
		cancel()
		t.Skipf("docker daemon not reachable: %v", err)
	}
	cancel()
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("closing docker client: %v", err)
		}
	})

	const serviceName = "levelrail-test-app-controller"
	image1 := "nginx:alpine"
	image2 := "nginx:alpine-levelrail-test-redeploy" // a local retag, not a network pull

	longCtx := context.Background()
	if err := pullIfMissing(longCtx, t, rawCli, image1); err != nil {
		t.Fatalf("pull %s: %v", image1, err)
	}
	if err := rawCli.ImageTag(longCtx, image1, image2); err != nil {
		t.Fatalf("tag %s as %s: %v", image1, image2, err)
	}

	cleanupContainers(longCtx, t, rt, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, rt, serviceName) })

	db := openLiveStore(t)

	desired := store.DesiredService{
		Name: serviceName, Image: image1, Port: 80,
		Health: &store.ServiceHealth{
			Readiness: &store.ServiceProbe{Path: "/", Interval: 200 * time.Millisecond, Timeout: 1 * time.Second},
		},
	}
	if err := db.SaveDesiredService(longCtx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	ctrl := New(serviceName, db, rt, WithReadyBudget(20*time.Second))

	// First reconcile: fresh deploy.
	result, err := ctrl.Reconcile(longCtx)
	if err != nil {
		t.Fatalf("first Reconcile() error = %v, result = %+v", err, result)
	}
	if cond := conditionOf(t, result); cond.Status != "True" || cond.Reason != "Deployed" {
		t.Fatalf("first Reconcile() condition = %+v, want Status=True Reason=Deployed", cond)
	}

	firstTarget := containerName(serviceName, image1)
	state, err := rt.InspectByName(longCtx, firstTarget)
	if err != nil {
		t.Fatalf("InspectByName(%q) error = %v", firstTarget, err)
	}
	if state == nil || !state.Running {
		t.Fatalf("expected %q running after first Reconcile, got %+v", firstTarget, state)
	}

	// Second reconcile, same desired state: must be a no-op, not a
	// second create.
	before := containerCount(longCtx, t, rt, serviceName)
	if _, err := ctrl.Reconcile(longCtx); err != nil {
		t.Fatalf("second (no-op) Reconcile() error = %v", err)
	}
	after := containerCount(longCtx, t, rt, serviceName)
	if before != after {
		t.Errorf("container count changed on a no-op reconcile: %d -> %d", before, after)
	}

	// Redeploy: change the desired image, reconcile, and prove the
	// cutover actually happened, not just that Reconcile said it did.
	desired.Image = image2
	if err := db.SaveDesiredService(longCtx, desired); err != nil {
		t.Fatalf("SaveDesiredService() for redeploy error = %v", err)
	}

	result, err = ctrl.Reconcile(longCtx)
	if err != nil {
		t.Fatalf("redeploy Reconcile() error = %v, result = %+v", err, result)
	}
	if cond := conditionOf(t, result); cond.Status != "True" || cond.Reason != "Deployed" {
		t.Fatalf("redeploy Reconcile() condition = %+v, want Status=True Reason=Deployed", cond)
	}

	secondTarget := containerName(serviceName, image2)
	newState, err := rt.InspectByName(longCtx, secondTarget)
	if err != nil {
		t.Fatalf("InspectByName(%q) error = %v", secondTarget, err)
	}
	if newState == nil || !newState.Running {
		t.Fatalf("expected new container %q running after redeploy, got %+v", secondTarget, newState)
	}

	oldState, err := rt.InspectByName(longCtx, firstTarget)
	if err != nil {
		t.Fatalf("InspectByName(%q) after redeploy error = %v", firstTarget, err)
	}
	if oldState != nil {
		t.Errorf("expected old container %q removed after successful redeploy cutover, still found: %+v", firstTarget, oldState)
	}
}

func pullIfMissing(ctx context.Context, t *testing.T, cli *dockerclient.Client, ref string) error {
	t.Helper()
	_, _, err := cli.ImageInspectWithRaw(ctx, ref)
	if err == nil {
		return nil
	}
	rc, err := cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	_, err = io.Copy(io.Discard, rc)
	return err
}

func cleanupContainers(ctx context.Context, t *testing.T, rt docker.Runtime, prefix string) {
	t.Helper()
	found, err := rt.ListByPrefix(ctx, prefix)
	if err != nil {
		return
	}
	for _, cs := range found {
		_ = rt.Stop(ctx, cs.ID, 3*time.Second)
		_ = rt.Remove(ctx, cs.ID, true)
	}
}

func containerCount(ctx context.Context, t *testing.T, rt docker.Runtime, prefix string) int {
	t.Helper()
	found, err := rt.ListByPrefix(ctx, prefix)
	if err != nil {
		t.Fatalf("ListByPrefix() error = %v", err)
	}
	return len(found)
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
