package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_Reconcile_Redis_Live is the real end-to-end proof for
// this controller: a real store, a real Docker daemon, a real named
// volume, and a real running Redis container, verified by inspecting the
// container's mounts through the raw Docker Engine API directly rather
// than trusting this package's own return values, the same
// independent-verification rigor internal/docker and
// internal/reconcile/application's live tests already establish.
func TestController_Reconcile_Redis_Live(t *testing.T) {
	rt, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	rawCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	if _, err := rawCli.Ping(pingCtx); err != nil {
		cancel()
		t.Skipf("docker daemon not reachable: %v", err)
	}
	cancel()
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("closing docker client: %v", err)
		}
	})

	const dbName = "levelrail-test-redis-db"
	ctx := context.Background()

	target := containerName(dbName)
	volName := dataVolumeName(dbName)

	cleanup := func() {
		if state, err := rt.InspectByName(ctx, target); err == nil && state != nil {
			_ = rt.Stop(ctx, state.ID, 3*time.Second)
			_ = rt.Remove(ctx, state.ID, true)
		}
		_ = rawCli.VolumeRemove(ctx, volName, true)
	}
	cleanup()
	t.Cleanup(cleanup)

	db := openLiveStore(t)
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{
		Name: dbName, Engine: store.EngineRedis, Version: "7-alpine",
	}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}

	ctrl := New(dbName, db, rt)

	// First reconcile: fresh deploy.
	result, err := ctrl.Reconcile(ctx)
	if err != nil {
		t.Fatalf("first Reconcile() error = %v, result = %+v", err, result)
	}
	if cond := conditionOf(t, result); cond.Status != "True" || cond.Reason != "Deployed" {
		t.Fatalf("first Reconcile() condition = %+v, want Status=True Reason=Deployed", cond)
	}

	state, err := rt.InspectByName(ctx, target)
	if err != nil {
		t.Fatalf("InspectByName(%q) error = %v", target, err)
	}
	if state == nil || !state.Running {
		t.Fatalf("expected %q running after first Reconcile, got %+v", target, state)
	}

	// Independent verification: inspect the raw container and the raw
	// volume directly through the Engine API, not through this
	// controller's or internal/docker's own return values.
	rawContainer, err := rawCli.ContainerInspect(ctx, state.ID)
	if err != nil {
		t.Fatalf("raw ContainerInspect() error = %v", err)
	}
	foundMount := false
	for _, m := range rawContainer.Mounts {
		if m.Name == volName && m.Destination == "/data" {
			foundMount = true
		}
	}
	if !foundMount {
		t.Errorf("expected volume %q mounted at /data in raw container mounts, got %+v", volName, rawContainer.Mounts)
	}
	if rawContainer.HostConfig.RestartPolicy.Name != "no" {
		t.Errorf("restart policy = %q, want \"no\": the reconciler owns restart, not Docker", rawContainer.HostConfig.RestartPolicy.Name)
	}

	rawVol, err := rawCli.VolumeInspect(ctx, volName)
	if err != nil {
		t.Fatalf("raw VolumeInspect() error = %v", err)
	}
	if rawVol.Name != volName {
		t.Errorf("volume name = %q, want %q", rawVol.Name, volName)
	}

	// Second reconcile, same desired state: must be a no-op, not a
	// second create.
	result, err = ctrl.Reconcile(ctx)
	if err != nil {
		t.Fatalf("second (no-op) Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Status != "True" || cond.Reason != "AlreadyRunning" {
		t.Fatalf("second Reconcile() condition = %+v, want Status=True Reason=AlreadyRunning", cond)
	}
	stillRunning, err := rt.InspectByName(ctx, target)
	if err != nil {
		t.Fatalf("InspectByName() after no-op reconcile error = %v", err)
	}
	if stillRunning == nil || stillRunning.ID != state.ID {
		t.Errorf("expected the same container to still exist after a no-op reconcile, got %+v (was %+v)", stillRunning, state)
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
