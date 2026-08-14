package e2e

import (
	"context"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestDatabase_Live_RedisReconcile is this package's e2e proof for managed
// database reconciliation, the counterpart to
// TestDeploy_Live_BuildToHTTPS for databases instead of applications: a
// real store.DesiredDatabase saved through the same store.Open path
// openLiveStore uses, a real database.Controller converging it, and a
// real, running, volume-backed Redis container verified directly against
// the raw Docker Engine API, not against this test's own controller
// return value or internal/docker's return value alone.
//
// internal/reconcile/database/controller_live_test.go already proves the
// controller in isolation, including the no-op second-reconcile case;
// this test's job is narrower and complementary: prove the controller
// composes with a real on-disk store the same way this package's other
// live tests prove application reconciliation composes with build and
// ingress, closing the gap noted in TASKS.md's Phase 1 section that only
// applications had e2e coverage before this file.
//
// Deliberately out of scope, matching this package's other live tests
// and internal/reconcile/database/controller.go's own doc comment:
// Postgres (the controller refuses to start it until TASKS.md 1.7
// supplies credentials), multi-node placement, and backups. No ingress
// either: a database is not routed by Caddy, so there's no HTTPS leg
// here the way TestDeploy_Live_BuildToHTTPS has one.
func TestDatabase_Live_RedisReconcile(t *testing.T) {
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

	runtime, err := docker.NewClient()
	if err != nil {
		t.Fatalf("docker.NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("closing docker.Client: %v", err)
		}
	})

	// A name prefix distinct from this package's other e2e tests
	// ("levelrail-test-e2e-deploy" in deploy_test.go,
	// "levelrail-test-webhook-push" in webhook_test.go) and from
	// internal/reconcile/database/controller_live_test.go's own
	// "levelrail-test-redis-db", since all of these can run concurrently
	// against the same shared local Docker daemon.
	const dbName = "levelrail-test-e2e-db-redis"

	// target and volName mirror internal/reconcile/database/controller.go's
	// own unexported containerName and dataVolumeName functions exactly
	// ("db-" + name and "db-" + name + "-data"). They aren't exported, so
	// this test recomputes the same convention rather than reaching into
	// the package's internals.
	target := "db-" + dbName
	volName := "db-" + dbName + "-data"

	cleanup := func() {
		cleanupCtx := context.Background()
		if state, inspectErr := runtime.InspectByName(cleanupCtx, target); inspectErr == nil && state != nil {
			_ = runtime.Stop(cleanupCtx, state.ID, 3*time.Second)
			_ = runtime.Remove(cleanupCtx, state.ID, true)
		}
		_ = dockerCli.VolumeRemove(cleanupCtx, volName, true)
	}
	cleanup()
	t.Cleanup(cleanup)

	dbStore := openLiveStore(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Step 1: save desired state through the real store, the same
	// store.Open-backed temp-file DB deploy_test.go and webhook_test.go
	// use via openLiveStore, not an in-memory fake.
	if err := dbStore.SaveDesiredDatabase(ctx, store.DesiredDatabase{
		Name:    dbName,
		Engine:  store.EngineRedis,
		Version: "7",
	}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}

	// Step 2: a real database.Controller converges a real container.
	ctrl := database.New(dbName, dbStore, runtime)
	result, err := ctrl.Reconcile(ctx)
	if err != nil {
		t.Fatalf("database Controller.Reconcile() error = %v, result = %+v", err, result)
	}
	if len(result.Conditions) == 0 || result.Conditions[0].Status != "True" {
		t.Fatalf("database Controller.Reconcile() result = %+v, want a True Ready condition", result)
	}
	if result.Conditions[0].Reason != "Deployed" {
		t.Fatalf("database Controller.Reconcile() reason = %q, want %q (a fresh deploy)", result.Conditions[0].Reason, "Deployed")
	}

	// Step 3: independent verification, not trusting the controller's own
	// return value, the same rigor this package's other live tests apply.
	// First through docker.Runtime itself.
	containers, err := runtime.ListByPrefix(ctx, target)
	if err != nil {
		t.Fatalf("ListByPrefix(%q) error = %v", target, err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected exactly 1 container named %q after reconcile, got %d: %+v", target, len(containers), containers)
	}
	if !containers[0].Running {
		t.Fatalf("container %+v is not running", containers[0])
	}
	if containers[0].Image != "redis:7" {
		t.Fatalf("running container's image = %q, want %q", containers[0].Image, "redis:7")
	}

	state, err := runtime.InspectByName(ctx, target)
	if err != nil {
		t.Fatalf("InspectByName(%q) error = %v", target, err)
	}
	if state == nil || !state.Running {
		t.Fatalf("InspectByName(%q) = %+v, want a running container", target, state)
	}

	// Then through the raw Engine API directly, bypassing internal/docker
	// entirely, to prove the volume mount is real and not just an
	// artifact of how internal/docker happens to report state: the same
	// pattern internal/reconcile/database/controller_live_test.go and
	// internal/docker/client_live_test.go both already use.
	rawContainer, err := dockerCli.ContainerInspect(ctx, state.ID)
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
		t.Fatalf("expected volume %q mounted at /data in raw container mounts, got %+v", volName, rawContainer.Mounts)
	}

	rawVol, err := dockerCli.VolumeInspect(ctx, volName)
	if err != nil {
		t.Fatalf("raw VolumeInspect(%q) error = %v: expected a real named Docker volume to back this database's data", volName, err)
	}
	if rawVol.Name != volName {
		t.Fatalf("volume name = %q, want %q", rawVol.Name, volName)
	}
}
